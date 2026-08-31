package app

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tui/tips"
)

func TestCommandTipShowsWhileTypingCommand(t *testing.T) {
	app := newTestApp(t)
	app.refreshChrome()

	// Typing "/sessions " (with an argument position) shows that command's
	// tip in the info line, with {placeholders} personalized.
	app.input.SetValue("/sessions ")
	app.refreshChrome()
	got := app.infoLine.Text()
	if !strings.Contains(got, "search") {
		t.Fatalf("expected the /sessions tip in the info line, got: %q", got)
	}
}

func TestCommandTipPersonalized(t *testing.T) {
	app := newTestApp(t)
	app.input.SetValue("/artifact ")
	app.refreshChrome()
	got := app.infoLine.Text()
	// The personalized variant carries the live artifact count in place of
	// the {artifacts} placeholder.
	if strings.Contains(got, "{artifacts}") {
		t.Fatalf("placeholder not resolved: %q", got)
	}
	if !strings.Contains(got, "artifact") {
		t.Fatalf("tip missing: %q", got)
	}
}

func TestNoCommandTipForPlainText(t *testing.T) {
	app := newTestApp(t)
	app.input.SetValue("just a question")
	app.refreshChrome()
	if got := app.infoLine.Text(); strings.Contains(got, "·") && strings.HasPrefix(got, "just") {
		t.Fatalf("plain text must not produce a command tip: %q", got)
	}
	// The state remains idle: either the static hints or a rotating tip:/fact:
	// entry — never a tip derived from the typed text.
	got := app.infoLine.Text()
	if !strings.Contains(got, "enter sends") && !isRotatingIdleLine(got) {
		t.Fatalf("expected the idle line, got %q", got)
	}
}

// isRotatingIdleLine reports whether the text is one of the idle rotator's
// entries (a per-command tip or a coding fact).
func isRotatingIdleLine(text string) bool {
	return strings.HasPrefix(text, "tip: /") || strings.HasPrefix(text, "fact: ")
}

func TestTipContextCommandTipWins(t *testing.T) {
	// The tips package honors CommandTip for idle and palette states.
	if got := tips.Info(tips.StateIdle, tips.Context{CommandTip: "custom"}); got != "custom" {
		t.Fatalf("idle must show CommandTip, got %q", got)
	}
	if got := tips.Info(tips.StatePaletteOpen, tips.Context{CommandTip: "custom"}); got != "custom" {
		t.Fatalf("palette must show CommandTip, got %q", got)
	}
	// Other states keep their own guidance.
	if got := tips.Info(tips.StateRunning, tips.Context{CommandTip: "custom"}); got == "custom" {
		t.Fatalf("running state must not be overridden by CommandTip, got %q", got)
	}
}

func TestIncompleteCommandNameShowsNoTipWhilePrefixTyping(t *testing.T) {
	app := newTestApp(t)
	app.input.SetValue("/sess")
	app.refreshChrome()
	// "sess" is not a registered command yet (no argument position, no exact
	// match), so the idle line stays: either the static hints or a rotating
	// entry — never a tip for the incomplete word.
	got := app.infoLine.Text()
	if !strings.Contains(got, "enter sends") && !isRotatingIdleLine(got) {
		t.Fatalf("incomplete command must keep the idle line, got %q", got)
	}
	if isRotatingIdleLine(got) && strings.Contains(got, "/sess") {
		t.Fatalf("incomplete command must not resolve to a tip: %q", got)
	}
}
