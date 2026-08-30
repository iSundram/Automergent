package commands

import (
	"fmt"
	"testing"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
)

func TestHandleModel(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
		wantErr bool
		setup   func(*mockHost)
		verify  func(*mockHost, *testing.T)
	}{
		{
			name:    "show current model when no args",
			args:    []string{},
			wantMsg: "Current model: gemini-3.6-flash",
			setup:   func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "gemini-3.6-flash") {
					t.Fatalf("expected model info message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name:    "switch model",
			args:    []string{"gemini-3.5-pro"},
			wantMsg: "Model switched to gemini-3.5-pro",
			setup: func(m *mockHost) {
				m.models = []ai.Model{{ID: "gemini-3.5-pro", ContextLimit: 1000000}, {ID: "gemini-3.6-flash", ContextLimit: 1000000}}
			},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.switchProviderCalls) != 1 || m.switchProviderCalls[0].model != "gemini-3.5-pro" {
					t.Fatalf("expected SwitchProvider called with new model, got: %v", m.switchProviderCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "gemini-3.5-pro") {
					t.Fatalf("expected model switch message, got: %v", m.systemMessages)
				}
				if m.persistProjectConfigCalls != 1 {
					t.Fatalf("expected PersistProjectConfig called")
				}
			},
		},
		{
			name:    "reset model to default",
			args:    []string{"reset"},
			wantMsg: "Model reset to default",
			setup: func(m *mockHost) {
				m.models = []ai.Model{{ID: "gemini-3.6-flash", ContextLimit: 1000000}}
			},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.switchProviderCalls) != 1 || m.switchProviderCalls[0].model != "gemini-3.6-flash" {
					t.Fatalf("expected SwitchProvider called with default model, got: %v", m.switchProviderCalls)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleModel(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleProvider(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name:  "show current provider when no args",
			args:  []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "google") {
					t.Fatalf("expected provider info message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name: "switch provider",
			args: []string{"google", "gemini-3.5-pro"},
			setup: func(m *mockHost) {
				m.models = []ai.Model{{ID: "gemini-3.5-pro", ContextLimit: 1000000}}
			},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.switchProviderCalls) != 1 || m.switchProviderCalls[0].provider != "google" {
					t.Fatalf("expected SwitchProvider called, got: %v", m.switchProviderCalls)
				}
			},
		},
		{
			name:  "unknown provider shows error",
			args:  []string{"unknown"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "Unknown provider") {
					t.Fatalf("expected error for unknown provider, got: %v", m.errorMessages)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleProvider(m, tt.args)
			tt.verify(m, t)
		})
	}
}

// The /mode sub-palette must offer every user-selectable mode — it once
// listed only "edit" (a deprecated alias) and "plan", hiding the other half.
func TestModeSubCommandsCoverAllModes(t *testing.T) {
	cmd := modeCommand()
	want := agent.AllModes()
	if len(cmd.SubCommands) != len(want) {
		t.Fatalf("sub-palette has %d modes, want %d", len(cmd.SubCommands), len(want))
	}
	have := map[string]bool{}
	for _, sub := range cmd.SubCommands {
		have[sub.Name] = true
		if sub.Description == "" {
			t.Fatalf("sub-command %q has no description", sub.Name)
		}
	}
	for _, m := range want {
		if !have[m] {
			t.Fatalf("sub-palette is missing mode %q", m)
		}
	}
	if have["edit"] {
		t.Fatal("sub-palette must not offer the deprecated \"edit\" alias")
	}
	for _, hint := range []string{cmd.ArgsHint} {
		if !contains(hint, "accept-edits") || !contains(hint, "auto") {
			t.Fatalf("ArgsHint must list all modes, got %q", cmd.ArgsHint)
		}
	}
}

func TestHandleMode(t *testing.T) {	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name:  "show current mode when no args",
			args:  []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "edit") {
					t.Fatalf("expected mode info message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name:  "switch to plan mode",
			args:  []string{"plan"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.setModeCalls) != 1 || m.setModeCalls[0] != "plan" {
					t.Fatalf("expected SetMode called with plan, got: %v", m.setModeCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "plan") {
					t.Fatalf("expected mode switch message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name:  "invalid mode shows error",
			args:  []string{"invalid"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "auto|plan") {
					t.Fatalf("expected error for invalid mode, got: %v", m.errorMessages)
				}
			},
		},
		{
			name: "persist failure is surfaced, not swallowed",
			args: []string{"auto"},
			setup: func(m *mockHost) {
				m.persistProjectConfigErr = fmt.Errorf("disk full")
			},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.setModeCalls) != 1 || m.setModeCalls[0] != "auto" {
					t.Fatalf("mode should still switch in-memory, got: %v", m.setModeCalls)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "not saved") {
					t.Fatalf("persist failure must be reported, got: %v", m.systemMessages)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleMode(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleContext(t *testing.T) {
	m := NewMockHost()
	handleContext(m, []string{})
	if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "Provider: google") {
		t.Fatalf("expected context summary, got: %v", m.systemMessages)
	}

	m.Reset()
	handleContext(m, []string{"detail"})
	if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "detail") {
		t.Fatalf("expected detail message, got: %v", m.systemMessages)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
