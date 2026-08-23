package components

import (
	"strings"
	"testing"
)

func TestBottomDockRenderAndCursor(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetWidth(80)
	if d.HasContent() || d.Height() != 0 {
		t.Fatal("empty dock must render nothing")
	}
	d.SetEntries([]DockEntry{
		{Kind: DockShell, ID: "shell-1", Label: "npm run dev", Status: "running", Detail: "12s"},
		{Kind: DockShell, ID: "shell-2", Label: "go test ./...", Status: "failed", Detail: "exit 1"},
		{Kind: DockAgent, ID: "agent-3", Label: "worker-1", Status: "running", Detail: "task · 4s"},
	})
	d.MoveCursor(2)
	if _, ok := d.Selected(); !ok {
		t.Fatal("cursor should be valid")
	}
	view := d.View()
	for _, want := range []string{"BACKGROUND", "npm run dev", "go test ./...", "worker-1", "running", "exit 1"} {
		if !strings.Contains(view, want) {
			t.Errorf("dock view missing %q:\n%s", want, view)
		}
	}
}

func TestBottomDockClampsAndFocus(t *testing.T) {
	d := NewBottomDock(testStyles())
	d.SetEntries([]DockEntry{{Kind: DockAgent, ID: "a", Label: "x", Status: "running"}})
	for i := 0; i < 5; i++ {
		d.MoveCursor(1)
	}
	_, ok := d.Selected()
	if !ok {
		t.Fatal("cursor must stay clamped on a real entry")
	}
	d.MoveCursor(-10)
	d.SetEntries(nil)
	d.SetFocused(true)
	if d.Focused() != true { // focus is orthogonal to content
		t.Fatal("focus should persist independent of entries")
	}
}
