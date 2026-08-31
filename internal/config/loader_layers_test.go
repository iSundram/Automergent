package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeYAML is a test helper that writes a config file, creating parents.
func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// config.local.yaml (personal, gitignored) overrides the committed project
// config, which overrides the global config.
func TestLayerPrecedence_LocalOverridesProject(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	writeYAML(t, filepath.Join(home, "config.yaml"), "verbose: false\n")
	writeYAML(t, filepath.Join(project, ".automergent", "config.yaml"), "verbose: false\n")
	writeYAML(t, filepath.Join(project, ".automergent", "config.local.yaml"), "verbose: true\n")

	l, err := NewLoader(&LoaderOptions{
		GlobalPath: filepath.Join(home, "config.yaml"),
		ProjectDir: project,
	})
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Verbose {
		t.Fatal("local layer did not override project layer")
	}
}

// The managed policy layer overrides every other layer, including env vars.
func TestLayerPrecedence_PolicyOverridesAll(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	policy := filepath.Join(home, "policy.yaml")

	writeYAML(t, filepath.Join(home, "config.yaml"), "verbose: true\n")
	writeYAML(t, policy, "verbose: false\n")

	t.Setenv("AUTOMERGENT_POLICY_CONFIG", policy)
	t.Setenv("AUTOMERGENT_VERBOSE", "true")

	l, err := NewLoader(&LoaderOptions{
		GlobalPath: filepath.Join(home, "config.yaml"),
		ProjectDir: project,
	})
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Verbose {
		t.Fatal("policy layer must override env and global layers")
	}

	// And the source of the winning key is the policy file.
	if src, ok := l.Sources()["verbose"]; !ok || src.Layer != LayerPolicy {
		t.Fatalf("source of verbose = %+v, want policy layer", src)
	}
}

// Without a policy config the layer is simply absent and nothing changes.
func TestLayerPolicy_AbsentByDefault(t *testing.T) {
	home := t.TempDir()
	writeYAML(t, filepath.Join(home, "config.yaml"), "verbose: true\n")
	t.Setenv("AUTOMERGENT_POLICY_CONFIG", filepath.Join(home, "missing.yaml"))

	l, err := NewLoader(&LoaderOptions{
		GlobalPath: filepath.Join(home, "config.yaml"),
		// An isolated ProjectDir avoids the cwd-walk picking up whatever
		// real .automergent/config.yaml exists on the test machine.
		ProjectDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Verbose {
		t.Fatal("missing policy file must be ignored")
	}
}
