package components

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tui/render"
)

func TestBottomDockRenderAndCursor(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetWidth(80)
	if d.HasContent() || d.Height() != 0 {
		t.Fatal("empty dock must render nothing")
	}
	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "shell-1", Label: "npm run dev", Status: render.StatusRunning, Started: time.Now()},
		{Kind: DockShell, ID: "shell-2", Label: "go test ./...", Status: render.StatusFailed, Activity: "exit 1"},
		{Kind: DockAgent, ID: "agent-3", Label: "worker-1", Status: render.StatusRunning, AgentKind: render.KindGeneral, Started: time.Now()},
	})
	d.MoveCursor(2)
	if _, ok := d.Selected(); !ok {
		t.Fatal("cursor should be valid")
	}
	// The tray only renders while focused — that is the rail/tray split.
	d.SetFocused(true)
	view := d.View()
	for _, want := range []string{"background", "npm run dev", "go test ./...", "worker-1", "exit 1"} {
		if !strings.Contains(view, want) {
			t.Errorf("dock view missing %q:\n%s", want, view)
		}
	}
}

func TestBottomDockClampsAndFocus(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetEntries([]DockEntry{{Kind: DockAgent, ID: "a", Label: "x", Status: render.StatusRunning}})
	for i := 0; i < 5; i++ {
		d.MoveCursor(1)
	}
	if _, ok := d.Selected(); !ok {
		t.Fatal("cursor must stay clamped on a real entry")
	}
	d.MoveCursor(-10)
	d.SetEntries(nil)
	d.SetFocused(true)
	if d.Focused() != true { // focus is orthogonal to content
		t.Fatal("focus should persist independent of entries")
	}
}

// Height must be a function of the entry count alone — the invariant that keeps
// the conversation from reflowing while background work runs. The same rows
// with wildly different content must cost exactly the same height.
func TestBottomDockHeightIsCountInvariant(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetWidth(80)
	d.SetFocused(true)
	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "s1", Label: "x", Status: render.StatusRunning},
		{Kind: DockShell, ID: "s2", Label: strings.Repeat("long ", 60), Status: render.StatusFailed, Activity: strings.Repeat("err ", 40)},
	})
	h := d.Height()
	v := d.View()
	if lines := strings.Count(strings.TrimRight(v, "\n"), "\n") + 1; lines != h {
		t.Errorf("Height() = %d but View() renders %d lines", h, lines)
	}

	// Same count, different content: height must not move.
	d.SetEntries([]DockEntry{
		{Kind: DockAgent, ID: "a1", Label: "tiny", Status: render.StatusDone, Depth: 2},
		{Kind: DockAgent, ID: "a2", Label: "also tiny", Status: render.StatusIdle},
	})
	if d.Height() != h {
		t.Errorf("height changed with content: %d -> %d", h, d.Height())
	}
}

// The rail is the always-on summary; it reports counts, never rows.
func TestBottomDockRailText(t *testing.T) {
	d := NewBottomDock(testStyles())
	if d.RailText() != "" {
		t.Fatalf("empty dock rail = %q, want empty", d.RailText())
	}
	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "s1", Label: "x", Status: render.StatusRunning},
		{Kind: DockShell, ID: "s2", Label: "y", Status: render.StatusRunning},
		{Kind: DockShell, ID: "s3", Label: "z", Status: render.StatusFailed},
	})
	if got := d.RailText(); got != "2 running · 1 failed" {
		t.Fatalf("RailText = %q", got)
	}
	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "s1", Label: "x", Status: render.StatusDone},
	})
	if got := d.RailText(); got != "1 finished" {
		t.Fatalf("RailText = %q", got)
	}
}
