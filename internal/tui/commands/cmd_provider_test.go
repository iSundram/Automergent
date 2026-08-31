package commands

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/config"
)

func TestHandleProviderStatus(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleProvider(m, []string{"status"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderList(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleProvider(m, []string{"list"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderUse(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantError bool
	}{
		{"valid provider", []string{"google"}, false},
		{"no args shows usage", []string{}, false},
		{"unknown provider", []string{"nonexistent"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockHost()
			m.provider = "google"
			m.model = "gemini-3.6-flash"
			handleProvider(m, tt.args)
			if (len(m.errorMessages) > 0) != tt.wantError {
				t.Errorf("errors = %v, wantError %v", m.errorMessages, tt.wantError)
			}
		})
	}
}

func TestHandleProviderUseWithModel(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleProvider(m, []string{"use", "google", "gemini-pro"})
	if m.model != "gemini-pro" {
		t.Errorf("model = %q, want gemini-pro", m.model)
	}
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderBackend(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleProvider(m, []string{"backend"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderBackendSet(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleProvider(m, []string{"backend", "vertex"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderSetup(t *testing.T) {
	m := NewMockHost()
	handleProvider(m, []string{"setup"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderTest(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleProvider(m, []string{"test"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderSet(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.apiKey = "test-key"
	handleProvider(m, []string{"set", "apiKey", "new-key"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderUnset(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.apiKey = "test-key"
	handleProvider(m, []string{"unset", "apiKey"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderFallback(t *testing.T) {
	m := NewMockHost()
	handleProvider(m, []string{"fallback"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleProviderNoArgs(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleProvider(m, []string{})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelList(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleModel(m, []string{"list"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelInfo(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleModel(m, []string{"info"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelSwitch(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleModel(m, []string{"gemini-pro"})
	if m.model != "gemini-pro" {
		t.Errorf("model = %q, want gemini-pro", m.model)
	}
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelSwitchNoArgs(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleModel(m, []string{})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelAdd(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	handleModel(m, []string{"add", "my-custom-model"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelRemove(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	// First add a custom model
	handleModel(m, []string{"add", "my-custom-model"})
	// Then remove it
	handleModel(m, []string{"remove", "my-custom-model"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelRefresh(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	handleModel(m, []string{"refresh"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestHandleModelReset(t *testing.T) {
	m := NewMockHost()
	m.provider = "google"
	m.model = "gemini-3.6-flash"
	m.defaultModel = "gemini-3.6-flash"
	handleModel(m, []string{"reset"})
	if len(m.errorMessages) != 0 {
		t.Fatalf("unexpected errors: %v", m.errorMessages)
	}
}

func TestProviderLoginVertex(t *testing.T) {
	t.Run("valid ADC with project configured", func(t *testing.T) {
		m := NewMockHost()
		m.vertexAuthOK = true
		m.vertexAuthDetail = "Application Default Credentials valid"
		m.providerConfigs["google-vertex"] = config.ProviderConfig{Project: "my-gcp", Location: "us-central1"}
		handleProvider(m, []string{"login", "google-vertex"})
		if len(m.systemMessages) != 1 {
			t.Fatalf("expected one message, got %d", len(m.systemMessages))
		}
		msg := m.systemMessages[0]
		for _, want := range []string{"✓ Application Default Credentials valid", "Project: my-gcp", "/provider test google-vertex"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("missing %q:\n%s", want, msg)
			}
		}
	})

	t.Run("missing ADC guides login and project", func(t *testing.T) {
		m := NewMockHost()
		m.vertexAuthOK = false
		m.vertexAuthDetail = "no valid Application Default Credentials — run: gcloud auth application-default login"
		handleProvider(m, []string{"login", "google-vertex"})
		msg := m.systemMessages[0]
		for _, want := range []string{"✗ no valid", "gcloud auth application-default login", "--project <gcp-project>"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("missing %q:\n%s", want, msg)
			}
		}
	})
}

func TestProviderLoginAIStudio(t *testing.T) {
	t.Run("key present points at test", func(t *testing.T) {
		m := NewMockHost()
		m.authSources["google-aistudio"] = "config"
		handleProvider(m, []string{"login", "google-aistudio"})
		msg := m.systemMessages[0]
		if !strings.Contains(msg, "✓ API key resolves from config") || !strings.Contains(msg, "/provider test") {
			t.Fatalf("wrong guidance:\n%s", msg)
		}
	})

	t.Run("no key guides setup", func(t *testing.T) {
		m := NewMockHost()
		m.authSources["google-aistudio"] = ""
		handleProvider(m, []string{"login", "google-aistudio"})
		msg := m.systemMessages[0]
		for _, want := range []string{"✗ No API key", "aistudio.google.com/apikey", "GOOGLE_API_KEY", "--api-key"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("missing %q:\n%s", want, msg)
			}
		}
	})

	t.Run("unknown provider errors", func(t *testing.T) {
		m := NewMockHost()
		handleProvider(m, []string{"login", "nope"})
		if len(m.errorMessages) != 1 {
			t.Fatalf("expected error, got %v", m.errorMessages)
		}
	})
}
