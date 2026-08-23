package app

// Global key routing.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"github.com/iSundram/Automergent/internal/tui/components"
	"strings"
	"time"
)

func (a *App) handleKey(m tea.KeyMsg) tea.Cmd {
	if a.showHelp {
		if m.String() == "?" || m.String() == "esc" || m.String() == "q" {
			a.showHelp = false
		}
		return nil
	}
	key := m.String()
	if key == "esc" && a.thinking {
		a.cancelActiveRun("Interrupted")
		return nil
	}
	if a.palette.Visible() {
		switch key {
		case "enter":
			if sel := a.palette.Selected(); sel != nil {
				if sel.Disabled {
					a.statusBar.SetStatus(sel.DisabledReason)
					return nil
				}
				trigger := a.input.TriggerType()
				if trigger == "command" || trigger == "help" {
					definition, known := a.commands.Lookup(sel.Value)
					if known && definition.Immediate {
						a.input.Reset()
						a.palette.Hide()
						a.layout()
						return a.handleSlashCommand("/" + sel.Value)
					}
					// Argument sub-palettes (model/provider/mode) are declared in
					// components.SlashSubPalettes — the same source TriggerType uses.
					if components.SlashSubPalettes[sel.Value] {
						a.input.InsertValue(sel.Value)
						a.updatePalette()
						a.palette.Show(a.palette.Items(), a.input.TriggerValue())
						a.layout()
						if sel.Value == "model" && len(a.availableModels) == 0 {
							return a.fetchModels()
						}
						return nil
					}
				}
				a.input.InsertValue(sel.Value)
				a.palette.Hide()
				a.layout()
				return nil
			}
		case "up", "down", "ctrl+p", "ctrl+n", "tab", "shift+tab", "ctrl+tab", "pgup", "pgdown":
			pal, cmd := a.palette.Update(m)
			a.palette = pal
			return cmd
		case "esc":
			a.palette.Hide()
			a.layout()
			return nil
		}
	}
	switch key {
	case "ctrl+c":
		now := time.Now()
		if now.Sub(a.lastCtrlCAt) <= time.Second {
			a.cancel()
			return tea.Quit
		}
		a.lastCtrlCAt = now
		if a.thinking {
			a.cancelActiveRun("Interrupted")
		} else {
			a.statusBar.SetStatus("Press Ctrl+C again to exit")
			return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return clearCtrlCStatusMsg{}
			})
		}
		return nil
	case "ctrl+q":
		a.cancel()
		return tea.Quit
	case "ctrl+d":
		return a.dispatchByName("diff")
	case "ctrl+l":
		a.lspPanel.Toggle()
		a.layout()
		return nil
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
		if a.taskBoard.Visible() {
			a.refreshTaskBoard()
		}
		a.layout()
		return nil
	case "ctrl+t":
		return a.dispatchByName("tree")
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
		if (key == "enter" || key == "ctrl+m") && !a.thinking {
			prompt := strings.TrimSpace(a.input.Value())
			if prompt != "" {
				// If we are waiting for an ask_user response, send it
				if a.askUserReplyCh != nil {
					a.askUserReplyCh <- prompt
					a.askUserReplyCh = nil
					a.input.Reset()
					a.statusBar.SetStatus("Thinking…")
					return nil
				}

				a.input.Reset()
				a.palette.Hide()
				a.layout()
				if strings.HasPrefix(prompt, "/") {
					return a.handleSlashCommand(prompt)
				}
				return a.startAgent(prompt)
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
