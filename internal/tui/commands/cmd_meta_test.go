package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesProjectMemoryOnce(t *testing.T) {
	dir := t.TempDir()
	m := NewMockHost()
	m.workDir = dir

	if res := handleInit(m, nil); res.Cmd != nil {
		t.Fatalf("init should not schedule async work: %#v", res)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AUTOMERGENT.md"))
	if err != nil {
		t.Fatalf("template not written: %v", err)
	}
	if !strings.Contains(string(data), "# AUTOMERGENT.md") || !strings.Contains(string(data), "## Build & Test") {
		t.Fatalf("unexpected template:\n%s", data)
	}

	first := string(data)
	handleInit(m, nil) // second run must not overwrite
	data2, _ := os.ReadFile(filepath.Join(dir, "AUTOMERGENT.md"))
	if string(data2) != first {
		t.Fatal("existing AUTOMERGENT.md was overwritten")
	}
	if len(m.systemMessages) < 2 || !strings.Contains(m.systemMessages[1], "already exists") {
		t.Fatalf("expected skip message, got %v", m.systemMessages)
	}
}

func TestInitWithoutWorkDirFails(t *testing.T) {
	m := NewMockHost()
	m.workDir = ""
	handleInit(m, nil)
	if len(m.errorMessages) == 0 {
		t.Fatal("expected error when workdir is empty")
	}
}

func TestDoctorPassesOnHealthyHost(t *testing.T) {
	m := NewMockHost()
	m.workDir = t.TempDir()

	handleDoctor(m, nil)
	out := strings.Join(m.systemMessages, "\n")
	if strings.Contains(out, "✗") {
		t.Fatalf("healthy host should have no failures:\n%s", out)
	}
	if !strings.Contains(out, "✓ API key (google): set") {
		t.Fatalf("missing api key check:\n%s", out)
	}
	last := m.statusMessages[len(m.statusMessages)-1]
	if !strings.Contains(last, "all checks passed") {
		t.Fatalf("unexpected status: %q", last)
	}
}

func TestDoctorReportsIssues(t *testing.T) {
	m := NewMockHost()
	m.configProblems = []string{"providers.google.api_key: required"}
	m.storageErr = errors.New("storage dir unwritable")
	m.sandboxAvailable = false
	m.sandboxKind = "namespaces"

	handleDoctor(m, nil)
	out := strings.Join(m.systemMessages, "\n")
	for _, want := range []string{"✗ Configuration", "✗ Session storage", "! Sandbox"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
	if last := m.statusMessages[len(m.statusMessages)-1]; !strings.Contains(last, "issue") {
		t.Fatalf("status should mention issues, got %q", last)
	}
}

func TestDoctorTreatsDisabledSandboxAsInformational(t *testing.T) {
	m := NewMockHost()
	m.sandboxKind = "off"

	handleDoctor(m, nil)
	out := strings.Join(m.systemMessages, "\n")
	if strings.Contains(out, "! Sandbox") || strings.Contains(out, "✗ Sandbox") {
		t.Fatalf("disabled sandbox should be informational:\n%s", out)
	}
}

func TestEnvShowsCoreDetails(t *testing.T) {
	m := NewMockHost()
	m.workDir = "/tmp/proj"
	m.globalConfigPath = "/home/u/.automergent/config.yaml"

	handleEnv(m, nil)
	out := m.systemMessages[0]
	for _, want := range []string{"test-1.0", "/tmp/proj", "sess-test", "google (model gemini-3.6-flash)", "/home/u/.automergent/config.yaml"} {
		if !strings.Contains(out, want) {
			t.Fatalf("env output missing %q:\n%s", want, out)
		}
	}
}

func TestVersionPrintsVersion(t *testing.T) {
	m := NewMockHost()
	handleVersion(m, nil)
	if !strings.Contains(m.systemMessages[0], "Automergent test-1.0") {
		t.Fatalf("unexpected version output: %q", m.systemMessages[0])
	}
}
