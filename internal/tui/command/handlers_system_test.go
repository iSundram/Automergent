package command

import (
	"testing"
)

func TestHandleStats(t *testing.T) {
	m := NewMockHost()
	handleStats(m, []string{})
	if m.showStatsCalls != 1 {
		t.Fatalf("expected ShowStats called, got %d", m.showStatsCalls)
	}
}

func TestHandleHelp(t *testing.T) {
	m := NewMockHost()
	handleHelp(m, []string{})
	if m.showHelpCalls != 1 {
		t.Fatalf("expected ShowHelp called, got %d", m.showHelpCalls)
	}
}

func TestHandleQuit(t *testing.T) {
	m := NewMockHost()
	cmd := handleQuit(m, []string{})
	if cmd == nil {
		t.Fatal("expected tea.Quit command")
	}
}