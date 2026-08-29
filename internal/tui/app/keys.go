package app

// Global key routing.
//
// ESC and Ctrl+C are delegated to escape.go, which owns one ordered precedence
// chain each. Everything else is dispatched here. The footer's hint row is
// derived from the same precedence (uistate.go), so an advertised key always
// does what the hint says.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (a *App) handleKey(m tea.KeyMsg) tea.Cmd {
	key := m.String()

	// ESC and Ctrl+C resolve through their own chains, ahead of every other
	// binding, so no pane can accidentally shadow them.
	switch key {
	case "esc":
		cmd := a.handleEscape()
		a.refreshChrome()
		return cmd
	case "ctrl+c":
		cmd := a.handleCtrlC()
		a.refreshChrome()
		return cmd
	}

	// Any other key disarms the pending double-press confirmations: an armed
	// "esc again to clear" must not survive the user typing something else.
	a.disarmEscape()
	a.ctrlCArmed = false
	defer a.refreshChrome()

	if a.showHelp {
		if key == "?" || key == "q" {
			a.showHelp = false
		}
		return nil
	}

	if a.palette.Visible() {
		switch key {
		case "enter":
			// All palette-enter semantics live in paletteEnter's rule table
			// (command_glue.go): dispatch vs insert vs argument completion.
			return a.paletteEnter()
		case "up", "down", "ctrl+p", "ctrl+n", "tab", "shift+tab", "ctrl+tab", "pgup", "pgdown":
			pal, cmd := a.palette.Update(m)
			a.palette = pal
			return cmd
		}
	}

	switch key {
	case "ctrl+q":
		a.cancel()
		return tea.Quit
	case "ctrl+d":
		return a.dispatchByName("diff")
	case "ctrl+s":
		// Same flow as /sessions: list, show browser, swallow the opening key.
		a.showSessions()
		return nil
	case "ctrl+r":
		return a.dispatchByName("review-mode")
	case "ctrl+u":
		a.input.SetValue("")
		a.updateActiveTokens()
		return nil
	case "ctrl+e":
		label := a.conversation.CycleExpand()
		a.statusBar.SetStatus(label)
		return nil
	case "ctrl+g":
		a.openEditReview()
		return nil
	case "ctrl+b":
		a.taskBoard.Toggle()
		a.layout()
		return nil
	case "ctrl+t":
		return a.dispatchByName("tree")
	case "ctrl+w":
		// IDE-style modified-files view: open it when hidden, otherwise the
		// fullscreen diff routes this key to cycle to the next tab.
		if !a.diffPane.Visible() {
			if n := a.diffPane.TabCount(); n == 0 {
				a.statusBar.SetStatus("No modified files yet")
				return nil
			} else {
				a.diffPane.Show()
				a.layout()
				a.statusBar.SetStatus(fmt.Sprintf("%d modified file(s)", n))
				return nil
			}
		}
		diff, cmd := a.diffPane.Update(m)
		a.diffPane = diff
		return cmd
	case "end":
		// Jump to the newest output. Only claimed when the view is actually
		// behind — otherwise the key falls through to the textarea, where `end`
		// means end-of-line and taking it would be a regression.
		if !a.conversation.AtBottom() {
			a.conversation.GotoEnd()
			return nil
		}
	case "ctrl+j":
		// Send a message at the next tool boundary instead of waiting for the
		// whole turn to finish.
		if a.thinking {
			if a.markQueueBoundary() {
				a.layout()
				return nil
			}
			a.setTransientNotice("nothing to send · type a message first")
			return nil
		}
		// Idle: ctrl+j is a plain send, so the key means the same thing in both
		// states rather than silently doing nothing.
		return a.submitInput()
	case "shift+tab":
		// Cycle the approval mode. The palette claims shift+tab while open, so
		// this is only reached when it is closed.
		return a.cycleMode()
	case "f1":
		return a.dispatchByName("help")
	case "f2":
		return a.dispatchByName("diff")
	case "tab":
		if !a.palette.Visible() {
			switch a.focus {
			case "input":
				a.focus = "conversation"
			case "conversation":
				if a.diffPane.Visible() {
					a.focus = "diff"
				} else if a.showFileTree {
					a.focus = "tree"
				} else {
					a.focus = "input"
				}
			case "diff":
				if a.showFileTree {
					a.focus = "tree"
				} else {
					a.focus = "input"
				}
			case "tree":
				a.focus = "input"
			}
			a.browsing = a.focus == "conversation"
			a.conversation.SetBrowsing(a.browsing)
			a.statusBar.SetBrowsing(a.browsing)
			a.layout()
			if a.focus == "input" {
				return a.input.Focus()
			}
			a.input.Blur()
			a.diffPane.Focus(a.focus == "diff")
		}
		return nil
	}

	switch a.focus {
	case "input":
		if key == "enter" || key == "ctrl+m" {
			if cmd, handled := a.handleSubmitKey(); handled {
				return cmd
			}
		}
		inp, cmd := a.input.Update(m)
		a.input = inp
		a.updateActiveTokens()
		trigger := a.input.TriggerType()
		if trigger != "" {
			// Opening the palette is the natural moment to pick up custom
			// command files edited mid-session (hot reload).
			if !a.palette.Visible() && (trigger == "command" || trigger == "help") {
				a.refreshCustomCommands()
			}
			a.updatePalette()
			a.palette.Show(a.palette.Items(), a.input.TriggerValue())
			a.layout()
			if trigger == "model" && len(a.availableModels) == 0 {
				return a.fetchModels()
			}
		} else if a.palette.Visible() {
			a.palette.Hide()
			a.layout()
		}
		return cmd
	case "conversation":
		conv, cmd := a.conversation.Update(m)
		a.conversation = conv
		return cmd
	case "diff":
		diff, cmd := a.diffPane.Update(m)
		a.diffPane = diff
		return cmd
	case "tree":
		tree, cmd := a.fileTree.Update(m)
		a.fileTree = tree
		return cmd
	}
	return nil
}

// handleSubmitKey handles Enter in the prompt. Reports whether the key was
// consumed; when it was not, the keystroke falls through to the textarea (so
// Enter on an empty prompt still inserts a newline where that is wanted).
func (a *App) handleSubmitKey() (tea.Cmd, bool) {
	prompt := strings.TrimSpace(a.input.Value())
	if prompt == "" {
		return nil, false
	}

	// A run in flight: queue instead of dropping the message on the floor.
	if a.thinking {
		// An ask_user prompt is the exception — the agent is blocked waiting
		// for exactly this, so it is delivered immediately.
		if a.askUserReplyCh != nil {
			a.askUserReplyCh <- prompt
			a.askUserReplyCh = nil
			a.input.Reset()
			a.statusBar.SetStatus("Thinking…")
			a.layout()
			return nil, true
		}
		a.enqueueMessage(prompt, false)
		a.input.Reset()
		a.palette.Hide()
		a.updateActiveTokens()
		a.layout()
		return nil, true
	}

	return a.submitInput(), true
}

// submitInput sends the current prompt: locally for slash commands, to the
// agent otherwise.
func (a *App) submitInput() tea.Cmd {
	prompt := strings.TrimSpace(a.input.Value())
	if prompt == "" {
		return nil
	}
	if a.askUserReplyCh != nil {
		a.askUserReplyCh <- prompt
		a.askUserReplyCh = nil
		a.input.Reset()
		a.statusBar.SetStatus("Thinking…")
		a.layout()
		return nil
	}

	a.input.Reset()
	a.palette.Hide()
	a.updateActiveTokens()
	a.layout()
	if strings.HasPrefix(prompt, "/") {
		// Only dispatch if it's a known command or has a space (sub-command).
		name := strings.Fields(prompt)[0]
		name = strings.TrimPrefix(name, "/")
		if _, known := a.commands.Lookup(name); known || strings.Contains(prompt, " ") {
			return a.handleSlashCommand(prompt)
		}
		// Unknown slash command — treat as regular message.
		return a.startAgent(prompt)
	}
	return a.startAgent(prompt)
}
