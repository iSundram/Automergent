package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestPromptSystemEnabledIsInSchema(t *testing.T) {
	field, ok := DefaultSchema().Fields["promptSystemEnabled"]
	if !ok {
		t.Fatal("promptSystemEnabled missing from schema")
	}
	if field.Type != TypeBool || field.Default != true {
		t.Fatalf("unexpected promptSystemEnabled schema: %+v", field)
	}

	cfg := Default()
	if err := SetConfigField(cfg, "promptSystemEnabled", false); err != nil {
		t.Fatal(err)
	}
	if cfg.PromptSystemEnabled {
		t.Fatal("SetConfigField did not apply false")
	}
}

func TestSaveIfLoadedDoesNotCreateMissingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Default()
	cfg.ConfigFile = path

	if err := cfg.SaveIfLoaded(); err != nil {
		t.Fatalf("SaveIfLoaded: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing config was created, stat error: %v", err)
	}
}

func TestSaveIfLoadedUpdatesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("mode: edit\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := Default()
	cfg.ConfigFile = path
	cfg.Mode = "plan"

	if err := cfg.SaveIfLoaded(); err != nil {
		t.Fatalf("SaveIfLoaded: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "mode: plan") {
		t.Fatalf("existing config was not updated: %s", data)
	}
}
