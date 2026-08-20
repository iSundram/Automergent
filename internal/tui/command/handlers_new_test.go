package command

import (
	"testing"
)

func TestHandleTheme(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name: "show current theme when no args",
			args: []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "modern") {
					t.Fatalf("expected theme list message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name: "switch to dracula theme",
			args: []string{"dracula"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.setThemeCalls) != 1 || m.setThemeCalls[0] != "dracula" {
					t.Fatalf("expected SetTheme called with dracula, got: %v", m.setThemeCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "dracula") {
					t.Fatalf("expected success message, got: %v", m.systemMessages)
				}
				if m.persistProjectConfigCalls != 1 {
					t.Fatalf("expected PersistProjectConfig called")
				}
			},
		},
		{
			name: "unknown theme shows error",
			args: []string{"unknown-theme"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "Unknown theme") {
					t.Fatalf("expected error for unknown theme, got: %v", m.errorMessages)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleTheme(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleKeybindings(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name: "show current keybindings when no args",
			args: []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "default") {
					t.Fatalf("expected keybindings list message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name: "switch to vim keybindings",
			args: []string{"vim"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.setKeybindingCalls) != 1 || m.setKeybindingCalls[0] != "vim" {
					t.Fatalf("expected SetKeybindings called with vim, got: %v", m.setKeybindingCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "vim") {
					t.Fatalf("expected success message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name: "unknown scheme shows error",
			args: []string{"unknown"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "Unknown keybinding") {
					t.Fatalf("expected error for unknown scheme, got: %v", m.errorMessages)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleKeybindings(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleCompact(t *testing.T) {
	m := NewMockHost()
	handleCompact(m, []string{})
	if m.compactContextCalls != 1 {
		t.Fatalf("expected CompactContext called, got %d", m.compactContextCalls)
	}
	if len(m.statusMessages) != 1 || m.statusMessages[0] != "Compacting context..." {
		t.Fatalf("expected status message, got: %v", m.statusMessages)
	}
}