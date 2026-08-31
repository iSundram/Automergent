package commands

import (
	"testing"

	"github.com/iSundram/Automergent/internal/config"
)

func TestHandleAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name:  "usage when no args",
			args:  []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.usageMessages) != 1 {
					t.Fatalf("expected usage message, got: %v", m.usageMessages)
				}
			},
		},
		{
			name: "update API key",
			args: []string{"new-api-key-123"},
			setup: func(m *mockHost) {
				m.providerConfigs["google"] = config.ProviderConfig{APIKey: "old"}
			},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.switchProviderCalls) != 1 {
					t.Fatalf("expected SwitchProvider called, got: %v", m.switchProviderCalls)
				}
				pc := m.ProviderConfig("google")
				if pc.APIKey != "new-api-key-123" {
					t.Fatalf("expected API key updated, got: %q", pc.APIKey)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "API key updated") {
					t.Fatalf("expected success message, got: %v", m.systemMessages)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleAPIKey(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name:  "usage when wrong arg count",
			args:  []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.usageMessages) != 1 {
					t.Fatalf("expected usage message")
				}
			},
		},
		{
			name:  "invalid URL shows error",
			args:  []string{"not-a-url"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "valid http") {
					t.Fatalf("expected URL validation error, got: %v", m.errorMessages)
				}
			},
		},
		{
			name:  "valid URL updates config",
			args:  []string{"https://example.com/v1"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.switchProviderCalls) != 1 {
					t.Fatalf("expected SwitchProvider called")
				}
				pc := m.ProviderConfig("google")
				if pc.BaseURL != "https://example.com/v1" {
					t.Fatalf("expected BaseURL updated, got: %q", pc.BaseURL)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleBaseURL(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleEffort(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		setup  func(*mockHost)
		verify func(*mockHost, *testing.T)
	}{
		{
			name:  "show current effort when no args",
			args:  []string{},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "high (default)") {
					t.Fatalf("expected effort info, got: %v", m.systemMessages)
				}
			},
		},
		{
			name:  "set effort to low",
			args:  []string{"low"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				pc := m.ProviderConfig("google")
				if pc.Effort != "low" {
					t.Fatalf("expected effort=low, got: %q", pc.Effort)
				}
				if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "low") {
					t.Fatalf("expected success message, got: %v", m.systemMessages)
				}
			},
		},
		{
			name:  "invalid effort shows error",
			args:  []string{"invalid"},
			setup: func(m *mockHost) {},
			verify: func(m *mockHost, t *testing.T) {
				if len(m.errorMessages) != 1 || !contains(m.errorMessages[0], "is not supported by") {
					t.Fatalf("expected error for invalid effort, got: %v", m.errorMessages)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			tt.setup(m)
			handleEffort(m, tt.args)
			tt.verify(m, t)
		})
	}
}

func TestHandleProviderAPIKey(t *testing.T) {
	m := NewMockHost()
	handleProviderAPIKey(m, []string{"google", "provider-key-123"})
	if len(m.systemMessages) != 1 || !contains(m.systemMessages[0], "google") {
		t.Fatalf("expected success message, got: %v", m.systemMessages)
	}
	pc := m.ProviderConfig("google")
	if pc.APIKey != "provider-key-123" {
		t.Fatalf("expected provider API key updated, got: %q", pc.APIKey)
	}
}

func TestHandleProviderBaseURL(t *testing.T) {
	m := NewMockHost()
	handleProviderBaseURL(m, []string{"google", "https://provider.example.com"})
	pc := m.ProviderConfig("google")
	if pc.BaseURL != "https://provider.example.com" {
		t.Fatalf("expected provider BaseURL updated, got: %q", pc.BaseURL)
	}
}
