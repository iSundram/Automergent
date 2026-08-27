package app

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tui/tips"
)

// The info area must never advertise an action the key no longer performs,
// and destructive transitions must never happen without a visible trace.

// TestEscArmHintExpires verifies that the armed double-ESC hint disappears
// once the clear window lapses: the info line must not keep saying
// "press esc again to clear" after that press would only re-arm.
func TestEscArmHintExpires(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("draft")

	app.handleKey(escKey)
	if got := app.infoLine.State(); got != tips.StateConfirmClear {
		t.Fatalf("after first esc, info state = %q, want %q", got, tips.StateConfirmClear)
	}

	// Simulate the window elapsing by backdating the arm timestamp, then
	// delivering the expiry message the tick produces.
	app.lastEscAt = time.Now().Add(-2 * escClearWindow)
	app.Update(clearEscArmMsg{})

	if app.escArmed {
		t.Error("armed state should be cleared after the window expires")
	}
	if got := app.infoLine.State(); got == tips.StateConfirmClear {
		t.Error("expired arm must not keep showing the confirm-clear hint")
	}

	// After expiry the next ESC re-arms (fresh window) instead of clearing.
	app.handleKey(escKey)
	if app.input.Value() != "draft" {
		t.Fatalf("esc after expiry must re-arm, not clear; input = %q", app.input.Value())
	}
	if got := app.infoLine.State(); got != tips.StateConfirmClear {
		t.Fatalf("re-armed esc should show the hint again, got %q", got)
	}
}

// TestInterruptDiscardingQueueIsAnnounced verifies that stopping a run with
// queued messages says how many were discarded instead of deleting silently.
func TestInterruptDiscardingQueueIsAnnounced(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.msgQueue = []queuedMessage{{text: "one"}, {text: "two"}}

	app.cancelActiveRun("Interrupted")

	view := strings.ToLower(app.statusBar.View())
	if !strings.Contains(view, "2 messages") || !strings.Contains(view, "discarded") {
		t.Errorf("status bar must announce discarded queue, got: %s", view)
	}
	if len(app.msgQueue) != 0 {
		t.Errorf("queue should be empty after cancel, got %d", len(app.msgQueue))
	}
}

// TestLegacyAskUserShowsAnswerHint verifies the legacy ask_user flow (reply
// channel pending without a questionnaire pane) advertises answering rather
// than queueing.
func TestLegacyAskUserShowsAnswerHint(t *testing.T) {
	app := sizedTestApp(t)
	ch := make(chan string, 1)
	app.askUserReplyCh = ch

	if got := app.uiState(); got != tips.StateAwaitingAnswer {
		t.Fatalf("uiState with pending ask_user channel = %q, want %q", got, tips.StateAwaitingAnswer)
	}
	info := tips.Info(tips.StateAwaitingAnswer, app.tipContext())
	if !strings.Contains(info, "agent asked") {
		t.Errorf("info line should explain the agent is waiting for an answer, got %q", info)
	}
}
