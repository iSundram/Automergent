package app

// ESC and Ctrl+C handling: one authoritative precedence chain each.
//
// ESC used to be handled in eight unrelated places with no defined order, so
// which one won depended on the order of unrelated `if` statements in
// handleKey. Typed input could never be cleared. Ctrl+C's double-press quit
// fired even mid-run, so hammering it to stop the agent killed the session
// instead.
//
// Both keys now resolve through a single ordered chain, and the footer hints
// are derived from the same precedence (see uistate.go), so what the hint row
// advertises is what the key will actually do.

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
)

// escClearWindow is how long the armed "press esc again to clear" state lasts.
// Short enough that a stray earlier ESC cannot silently arm a later clear.
const escClearWindow = 700 * time.Millisecond

// ctrlCQuitWindow is how long the armed "press ctrl+c again to exit" lasts.
const ctrlCQuitWindow = 2 * time.Second

// handleEscape resolves ESC against the current UI state. First match wins.
//
// The order is outermost-surface-first: ESC always dismisses the most recently
// opened thing, then falls back through focus owners, then the run, and only
// reaches the input text when nothing else claims it.
func (a *App) handleEscape() tea.Cmd {
	// Any ESC that is not the second half of a clear disarms the quit prompt.
	a.ctrlCArmed = false

	switch {
	// 1. Help overlay.
	case a.showHelp:
		a.showHelp = false
		a.disarmEscape()
		return nil

	// 2. Full-screen selector overlay.
	case a.selector.Visible():
		a.selector.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 3. Permission modal — declining is the modal's own business, so the key
	//    is forwarded rather than reimplemented here.
	case a.confirm.Visible():
		a.disarmEscape()
		return a.forwardEscape()

	// 4. Structured ask_user questionnaire.
	case a.questionnaire != nil && a.questionnaire.Visible():
		a.disarmEscape()
		return a.forwardEscape()

	// 4b. Fullscreen task inspector. Reachable only via handleInspectorKeys in
	//     normal flow; listed here so ESC can never fall through to the queue or
	//     the run while the inspector owns the screen.
	case a.inspector != nil && a.inspector.Visible():
		a.inspector.Hide()
		a.inspectorFilterMode = false
		a.disarmEscape()
		a.layout()
		return nil

	// 4c. Structured page viewer (full-page command output with actions).
	case a.pageViewer.Visible():
		a.pageViewer.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 4c'. Full-page command overlay.
	case a.fullPage.Visible():
		a.fullPage.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 4d. Provider Studio overlay.
	case a.providerStudio.Visible():
		a.providerStudio.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 4e. Model Hub overlay.
	case a.modelHub.Visible():
		a.modelHub.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 5. Diff overlay, including a proposal under review.
	case a.diffPane.Visible():
		if a.diffPane.Visible() {
			a.diffPane.Toggle()
		}
		a.reviewingProposal = ""
		a.disarmEscape()
		a.layout()
		return nil

	// 6. Inline session browser.
	case a.sessionBrowser.Visible():
		a.sessionBrowser.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 7. Completion palette. Typed text is deliberately kept: ESC here means
	//    "stop suggesting", not "throw away what I wrote".
	case a.palette.Visible():
		a.palette.Hide()
		a.disarmEscape()
		a.layout()
		return nil

	// 8. Background-task dock.
	case a.dockFocusActive():
		a.unfocusDock()
		a.disarmEscape()
		a.layout()
		return nil

	// 9. Conversation browsing returns focus to the prompt.
	case a.browsing:
		a.focus = "input"
		a.browsing = false
		a.conversation.SetBrowsing(false)
		a.statusBar.SetBrowsing(false)
		a.disarmEscape()
		a.layout()
		return a.input.Focus()

	// 10. A queue pending delivery is cleared before the run is touched: the
	//     user is more likely to mean "don't send that" than "stop everything".
	case len(a.msgQueue) > 0:
		n := a.clearQueue()
		a.disarmEscape()
		a.setTransientNotice(pluralMessages(n) + " unqueued")
		return nil

	// 11. Interrupt the active run.
	case a.thinking:
		a.cancelActiveRun("Interrupted")
		a.lastOutcome = outcomeInterrupted
		a.disarmEscape()
		return nil

	// 12. Clear typed input, on a confirmed second press.
	case a.input.Value() != "":
		if escArmActive(a.lastEscAt) {
			// Reset (not SetValue("")) pushes the text into input history, so
			// ctrl+p brings it back if the clear was a mistake.
			a.input.Reset()
			a.updateActiveTokens()
			a.disarmEscape()
			a.layout()
			return nil
		}
		a.escArmed = true
		a.lastEscAt = time.Now()
		// Expire the armed hint like ctrl+c does: without this, the info line
		// keeps saying "press esc again to clear" after the window closed and
		// the next esc silently re-arms instead of clearing.
		return tea.Tick(escClearWindow, func(time.Time) tea.Msg {
			return clearEscArmMsg{}
		})
	}

	// 13. Nothing to cancel.
	a.disarmEscape()
	if a.lastOutcome != outcomeNone {
		// Acknowledge a stale outcome badge rather than leaving it forever.
		a.lastOutcome = outcomeNone
		a.statusBar.SetStatus("Ready")
	}
	return nil
}

// forwardEscape hands ESC to whichever modal owns the keyboard, using the same
// Update path a normal key would take.
func (a *App) forwardEscape() tea.Cmd {
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	if a.confirm.Visible() {
		c, cmd := a.confirm.Update(msg)
		a.confirm = c
		if !a.confirm.Visible() {
			a.statusBar.ClearPermission()
			a.permissionTool = ""
			a.layout()
		}
		return cmd
	}
	if a.questionnaire != nil && a.questionnaire.Visible() {
		a.questionnaire.Update(msg)
		a.layout()
		return nil
	}
	return nil
}

// disarmEscape clears the armed double-ESC state.
func (a *App) disarmEscape() {
	a.escArmed = false
	a.lastEscAt = time.Time{}
}

// escArmActive reports whether the armed double-ESC window is still open.
// Both the ESC chain and the derived UI state gate on this, so the hint can
// never advertise a clear that the window no longer allows.
func escArmActive(armedAt time.Time) bool {
	return !armedAt.IsZero() && time.Since(armedAt) <= escClearWindow
}

// handleCtrlC resolves Ctrl+C against the current state.
//
// The rule that matters: while the agent is running, Ctrl+C interrupts and
// never arms a quit. A user pressing it repeatedly to make the agent stop must
// not lose the session as a side effect — they are told to stop the run first.
func (a *App) handleCtrlC() tea.Cmd {
	a.disarmEscape()

	// Running: interrupt on the first press, then refuse to escalate.
	if a.thinking {
		if a.ctrlCArmed {
			// Already interrupted once and still running (a tool mid-flight, or
			// a second press inside the same event batch): say what to do
			// rather than quitting.
			a.setTransientNotice("agent still running — press esc to stop it, then ctrl+c twice to exit")
			return nil
		}
		a.ctrlCArmed = true
		a.cancelActiveRun("Interrupted")
		a.lastOutcome = outcomeInterrupted
		return tea.Tick(ctrlCQuitWindow, func(time.Time) tea.Msg {
			return clearCtrlCStatusMsg{}
		})
	}

	// Idle with a queue: clearing it is less destructive than quitting.
	if len(a.msgQueue) > 0 {
		n := a.clearQueue()
		a.ctrlCArmed = false
		a.setTransientNotice(pluralMessages(n) + " unqueued")
		return nil
	}

	// Idle with typed text: clear the line, the readline convention.
	if a.input.Value() != "" {
		a.input.Reset()
		a.updateActiveTokens()
		a.ctrlCArmed = false
		a.layout()
		return nil
	}

	// Idle and empty: arm, then quit on confirmation.
	if a.ctrlCArmed {
		a.cancel()
		return tea.Quit
	}
	a.ctrlCArmed = true
	return tea.Tick(ctrlCQuitWindow, func(time.Time) tea.Msg {
		return clearCtrlCStatusMsg{}
	})
}

// cycleMode advances the approval mode (shift+tab) and persists the choice.
func (a *App) cycleMode() tea.Cmd {
	next := agent.NextMode(a.cfg.Mode)
	a.SetMode(next)
	if err := a.persistProjectConfig(); err != nil {
		a.setTransientNotice("mode: " + next + " (not saved: " + err.Error() + ")")
		return nil
	}
	a.setTransientNotice("mode: " + next + " — " + agent.ModeDescription(next))
	return nil
}

// setTransientNotice shows a short-lived message in the activity slot without
// disturbing the sticky outcome badge.
func (a *App) setTransientNotice(msg string) {
	a.statusBar.SetStatus(msg)
}

// pluralMessages renders "1 message" / "3 messages".
func pluralMessages(n int) string {
	if n == 1 {
		return "1 message"
	}
	return itoa(n) + " messages"
}

// itoa avoids pulling strconv into this file for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
