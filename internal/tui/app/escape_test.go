package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// escKey and ctrlCKey are the two keys whose precedence chains this file tests.
var (
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
	ctrlCKey = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// sizedTestApp returns an app with a usable terminal size, so layout() runs the
// same code paths it does in a real session.
func sizedTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.width, app.height = 120, 40
	app.layout()
	return app
}

// TestEscapeClearsInputOnSecondPress is the requested double-ESC behaviour:
// prefilled or typed text is only cleared once the user confirms.
func TestEscapeClearsInputOnSecondPress(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("some prefilled text")

	app.handleKey(escKey)
	if app.input.Value() != "some prefilled text" {
		t.Fatal("first ESC must not clear the input")
	}
	if !app.escArmed {
		t.Fatal("first ESC should arm the clear")
	}

	app.handleKey(escKey)
	if app.input.Value() != "" {
		t.Fatalf("second ESC should clear the input, got %q", app.input.Value())
	}
	if app.escArmed {
		t.Error("clear should disarm")
	}
}

// TestEscapeClearIsRestorable: clearing routes through Input.Reset so the text
// lands in history and ctrl+p brings it back. A destructive clear with no undo
// would be worse than not clearing at all.
func TestEscapeClearIsRestorable(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("recover me")
	app.handleKey(escKey)
	app.handleKey(escKey)
	if app.input.Value() != "" {
		t.Fatal("expected input cleared")
	}

	app.handleKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if got := app.input.Value(); got != "recover me" {
		t.Errorf("ctrl+p after clear = %q, want %q", got, "recover me")
	}
}

// TestEscapeArmExpires: a stray ESC minutes ago must not make a later single
// ESC destructive.
func TestEscapeArmExpires(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("keep me")

	app.handleKey(escKey)
	app.lastEscAt = time.Now().Add(-2 * escClearWindow) // window elapsed

	app.handleKey(escKey)
	if app.input.Value() != "keep me" {
		t.Error("ESC outside the arm window should re-arm, not clear")
	}
	if !app.escArmed {
		t.Error("the expired press should have re-armed")
	}
}

func TestNonEscapeKeyDisarmsEscape(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("text")
	app.handleKey(escKey)
	if !app.escArmed {
		t.Fatal("expected armed")
	}
	app.handleKey(tea.KeyPressMsg{Code: 'x'})
	if app.escArmed {
		t.Error("typing should disarm the pending clear")
	}
}

// TestEscapePrecedence walks the chain: each case sets up exactly one state and
// asserts ESC resolved that state and nothing else.
func TestEscapePrecedence(t *testing.T) {
	t.Run("help closes first", func(t *testing.T) {
		app := sizedTestApp(t)
		app.showHelp = true
		app.input.SetValue("untouched")
		app.handleKey(escKey)
		if app.showHelp {
			t.Error("ESC should close help")
		}
		if app.input.Value() != "untouched" {
			t.Error("closing help must not touch the input")
		}
	})

	t.Run("palette closes and keeps typed text", func(t *testing.T) {
		app := sizedTestApp(t)
		app.input.SetValue("/mod")
		app.updatePalette()
		app.palette.Show(app.palette.Items(), "mod")
		if !app.palette.Visible() {
			t.Skip("palette did not open in this configuration")
		}
		app.handleKey(escKey)
		if app.palette.Visible() {
			t.Error("ESC should hide the palette")
		}
		if app.input.Value() != "/mod" {
			t.Errorf("palette ESC must keep the typed text, got %q", app.input.Value())
		}
	})

	t.Run("diff closes before input clears", func(t *testing.T) {
		app := sizedTestApp(t)
		app.input.SetValue("kept")
		app.diffPane.SetContent("--- a\n+++ b\n")
		app.diffPane.Toggle()
		if !app.diffPane.Visible() {
			t.Skip("diff pane did not open")
		}
		app.handleKey(escKey)
		if app.diffPane.Visible() {
			t.Error("ESC should close the diff pane")
		}
		if app.input.Value() != "kept" {
			t.Error("closing the diff must not clear the input")
		}
	})

	t.Run("browsing returns focus to input", func(t *testing.T) {
		app := sizedTestApp(t)
		app.focus = "conversation"
		app.browsing = true
		app.handleKey(escKey)
		if app.browsing || app.focus != "input" {
			t.Errorf("ESC should return focus to the prompt, got focus=%q browsing=%v",
				app.focus, app.browsing)
		}
	})

	t.Run("queue clears before the run is interrupted", func(t *testing.T) {
		app := sizedTestApp(t)
		app.thinking = true
		app.enqueueMessage("queued work", false)
		app.handleKey(escKey)
		if len(app.msgQueue) != 0 {
			t.Error("ESC should clear the queue first")
		}
		if !app.thinking {
			t.Error("clearing the queue must not also interrupt the run")
		}
	})

	t.Run("running run is interrupted", func(t *testing.T) {
		app := sizedTestApp(t)
		app.thinking = true
		app.handleKey(escKey)
		if app.thinking {
			t.Error("ESC should interrupt the active run")
		}
		if app.lastOutcome != outcomeInterrupted {
			t.Errorf("outcome = %q, want %q", app.lastOutcome, outcomeInterrupted)
		}
	})

	t.Run("idle empty input is a no-op", func(t *testing.T) {
		app := sizedTestApp(t)
		cmd := app.handleKey(escKey)
		if cmd != nil {
			t.Error("ESC on an empty idle prompt should do nothing")
		}
		if app.escArmed {
			t.Error("nothing to clear, so nothing to arm")
		}
	})
}

// TestCtrlCDoesNotQuitWhileRunning is the headline safety property: a user
// hammering Ctrl+C to stop the agent must not lose the session.
func TestCtrlCDoesNotQuitWhileRunning(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true

	// First press interrupts.
	if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
		t.Fatal("first Ctrl+C while running must not quit")
	}
	if app.thinking {
		t.Error("first Ctrl+C should interrupt the run")
	}

	// Simulate a run still in flight (e.g. a tool mid-execution) and press
	// again — the old code quit here.
	app.thinking = true
	if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
		t.Fatal("second Ctrl+C while still running must not quit")
	}
	for i := 0; i < 5; i++ {
		app.thinking = true
		if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
			t.Fatalf("Ctrl+C press %d while running quit the session", i+3)
		}
	}
}

func TestCtrlCWhileRunningTellsUserToStopFirst(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.handleKey(ctrlCKey) // interrupts, arms nothing
	app.thinking = true     // still running
	app.handleKey(ctrlCKey)

	status := strings.ToLower(app.statusBar.Status())
	if !strings.Contains(status, "still running") {
		t.Errorf("status should explain the agent is still running, got %q", status)
	}
	if !strings.Contains(status, "esc") {
		t.Errorf("status should point at esc, got %q", status)
	}
}

func TestCtrlCQuitsWhenIdleAndEmpty(t *testing.T) {
	app := sizedTestApp(t)

	if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
		t.Fatal("first Ctrl+C should arm, not quit")
	}
	if !app.ctrlCArmed {
		t.Fatal("first Ctrl+C should arm the quit")
	}
	if cmd := app.handleKey(ctrlCKey); !isQuit(cmd) {
		t.Fatal("second Ctrl+C on an empty idle prompt should quit")
	}
}

func TestCtrlCClearsInputInsteadOfArmingQuit(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("half-typed thought")

	if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
		t.Fatal("Ctrl+C with text in the prompt must not quit")
	}
	if app.input.Value() != "" {
		t.Errorf("Ctrl+C should clear the prompt, got %q", app.input.Value())
	}
	if app.ctrlCArmed {
		t.Error("clearing the line should not arm a quit")
	}
}

func TestCtrlCClearsQueueBeforeQuitting(t *testing.T) {
	app := sizedTestApp(t)
	app.msgQueue = []queuedMessage{{text: "pending"}}

	if cmd := app.handleKey(ctrlCKey); isQuit(cmd) {
		t.Fatal("Ctrl+C with a queued message must not quit")
	}
	if len(app.msgQueue) != 0 {
		t.Error("Ctrl+C should clear the queue")
	}
}

func TestCtrlCArmExpiresViaTickMessage(t *testing.T) {
	app := sizedTestApp(t)
	app.handleKey(ctrlCKey)
	if !app.ctrlCArmed {
		t.Fatal("expected armed")
	}
	app.Update(clearCtrlCStatusMsg{})
	if app.ctrlCArmed {
		t.Error("the expiry tick should disarm the quit")
	}
}

func TestShiftTabCyclesMode(t *testing.T) {
	app := sizedTestApp(t)
	app.SetMode("manual")
	app.handleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if app.cfg.Mode == "manual" {
		t.Error("shift+tab should advance the mode")
	}
	if app.statusBar.Mode() != app.cfg.Mode {
		t.Errorf("footer chip %q out of sync with cfg.Mode %q", app.statusBar.Mode(), app.cfg.Mode)
	}
}

// isQuit reports whether a command is tea.Quit, by executing it and inspecting
// the message. Commands are opaque functions, so this is the only way to tell.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	return ok
}
