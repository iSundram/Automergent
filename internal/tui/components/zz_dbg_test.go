package components

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDebugKeyPress(t *testing.T) {
	m := tea.KeyPressMsg{Code: tea.KeyEnter}
	t.Logf("type assert ok? v=%q string=%q", func() any { _, ok := any(m).(tea.KeyMsg); return ok }(), m.String())
}
