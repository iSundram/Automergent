package config

import "testing"

func TestDefaultAndApplyFlags(t *testing.T) {
	cfg := Default()
	if cfg.Provider == "" || cfg.SessionDir == "" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.ReasoningPreAnalysis {
		t.Fatalf("expected reasoning pre-analysis default to be false")
	}

	cfg.ApplyFlags(&CLIFlags{Provider: "google", Model: "gemini-3.6-flash", NoSandbox: true, ContextFiles: []string{"a.md"}, APIKey: "secret", BaseURL: "https://example.com"})
	if cfg.Provider != "google" || cfg.Model != "gemini-3.6-flash" {
		t.Fatalf("flags not applied: %+v", cfg)
	}
	if cfg.Security.Sandbox != "off" {
		t.Fatalf("sandbox flag not applied: %+v", cfg.Security)
	}
	if cfg.Providers["google"].APIKey != "secret" || cfg.Providers["google"].BaseURL != "https://example.com" {
		t.Fatalf("provider overrides not applied: %+v", cfg.Providers["google"])
	}
}
