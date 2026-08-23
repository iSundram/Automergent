package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func newTestSelector(items []SelectorItem) SelectorOverlay {
	s := NewSelectorOverlay(themes.NewStyles(themes.Get("modern")))
	s.SetTitle("Test picker")
	s.SetSize(80, 24)
	s.SetItems(items)
	s.Show()
	return s
}

func TestSelectorEnterEmitsSelection(t *testing.T) {
	s := newTestSelector([]SelectorItem{{Label: "first"}, {Label: "second"}})
	s2, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s2.Visible() {
		t.Fatal("overlay should hide after selection")
	}
	if cmd == nil {
		t.Fatal("enter must emit a command")
	}
	msg := cmd()
	sel, ok := msg.(SelectorSelectedMsg)
	if !ok || sel.Index != 0 {
		t.Fatalf("expected SelectorSelectedMsg{0}, got %#v", msg)
	}
}

func TestSelectorEnterOnDisabledIsNoop(t *testing.T) {
	s := newTestSelector([]SelectorItem{{Label: "locked", Disabled: true, DisabledReason: "read only"}})
	s2, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !s2.Visible() {
		t.Fatal("disabled selection must keep overlay open")
	}
	if cmd != nil {
		t.Fatal("disabled selection must not emit a message")
	}
	if got := s2.SelectedDisabledReason(); got != "read only" {
		t.Fatalf("reason = %q", got)
	}
}

func TestSelectorEscHides(t *testing.T) {
	s := newTestSelector([]SelectorItem{{Label: "x"}})
	s2, _ := s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if s2.Visible() {
		t.Fatal("esc must close the overlay")
	}
}

func TestSelectorViewShowsStateMarkers(t *testing.T) {
	s := NewSelectorOverlay(themes.NewStyles(themes.Get("modern")))
	s.SetTitle("pick")
	s.SetSize(80, 24)
	s.SetItems([]SelectorItem{
		{Label: "current one", Detail: "active now", Current: true},
		{Label: "normal", Detail: "plain"},
		{Label: "off limits", Disabled: true, DisabledReason: "config managed"},
	})
	s.Show()
	view := s.View()
	for _, want := range []string{"PICK", "current one", "●", "·", "○", "config managed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestSelectorHiddenViewIsEmpty(t *testing.T) {
	s := NewSelectorOverlay(themes.NewStyles(themes.Get("modern")))
	s.SetSize(80, 24)
	if s.View() != "" {
		t.Fatal("hidden selector must render empty")
	}
}

func TestSelectorArrowsWrap(t *testing.T) {
	s := newTestSelector([]SelectorItem{{Label: "a"}, {Label: "b"}, {Label: "c"}})

	up := tea.KeyPressMsg{Code: tea.KeyUp}
	down := tea.KeyPressMsg{Code: tea.KeyDown}

	// From index 0 wrap up to last.
	s, _ = s.Update(up)
	if got := s.Cursor(); got != 2 {
		t.Fatalf("wrap-up cursor = %d, want 2", got)
	}
	// Wrap down back to first.
	s, _ = s.Update(down)
	s, _ = s.Update(down)
	if got := s.Cursor(); got != 1 {
		t.Fatalf("cursor = %d, want 1", got)
	}
}
