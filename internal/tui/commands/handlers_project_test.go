package commands

import (
	"testing"
)

func TestHandleTree(t *testing.T) {
	m := NewMockHost()
	handleTree(m, []string{})
	if m.toggleFileTreeCalls != 1 {
		t.Fatalf("expected ToggleFileTree called, got %d", m.toggleFileTreeCalls)
	}
	if len(m.statusMessages) != 1 || m.statusMessages[0] != "File tree toggled" {
		t.Fatalf("expected status message, got: %v", m.statusMessages)
	}
}

func TestHandleDiff(t *testing.T) {
	m := NewMockHost()
	handleDiff(m, []string{})
	if m.toggleDiffPaneCalls != 1 {
		t.Fatalf("expected ToggleDiffPane called, got %d", m.toggleDiffPaneCalls)
	}
}

func TestHandleLSP(t *testing.T) {
	m := NewMockHost()
	handleLSP(m, []string{})
	if m.toggleLSPPanelCalls != 1 {
		t.Fatalf("expected ToggleLSPPanel called, got %d", m.toggleLSPPanelCalls)
	}
}

func TestHandleSearch(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		verify func(*mockHost, *testing.T)
	}{
		{
			name: "usage when no query",
			args: []string{},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.usageMessages) != 1 {
					t.Fatalf("expected usage message")
				}
			},
		},
		{
			name: "search with query",
			args: []string{"needle"},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.searchWorkspaceCalls) != 1 || m.searchWorkspaceCalls[0] != "needle" {
					t.Fatalf("expected SearchWorkspace called with query, got: %v", m.searchWorkspaceCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "needle") {
					t.Fatalf("expected search results message, got: %v", m.systemMessages)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			handleSearch(m, tt.args)
			tt.verify(m, t)
		})
	}
}
