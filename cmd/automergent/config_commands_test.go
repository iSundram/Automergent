package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShowAndSourcesOutput(t *testing.T) {
	repoRoot := mustGetwd(t)
	homeDir := mustMkdirTempInRepo(t, repoRoot, "home-*")
	projectDir := mustMkdirTempInRepo(t, repoRoot, "project-*")
	t.Setenv("HOME", homeDir)
	t.Setenv("AUTOMERGENT_PROVIDER", "google")

	mustWriteFile(t, filepath.Join(homeDir, ".automergent", "config.yaml"), "provider: google\nmode: edit\n")
	mustWriteFile(t, filepath.Join(projectDir, ".automergent", "config.yaml"), "provider: google\n")

	restoreDir := mustChdir(t, projectDir)
	defer restoreDir()

	showOut, showErr, err := executeRootCommand(t, "config", "show")
	if err != nil {
		t.Fatalf("config show failed: %v\nstderr: %s", err, showErr)
	}
	if !strings.Contains(showOut, "Precedence: defaults < global < project < profile < env < session < cli") {
		t.Fatalf("missing precedence line in show output: %s", showOut)
	}
	if !strings.Contains(showOut, "provider") || !strings.Contains(showOut, "google") || !strings.Contains(showOut, "env(AUTOMERGENT_PROVIDER)") {
		t.Fatalf("show output missing provider effective/source details: %s", showOut)
	}
	if !strings.Contains(showOut, "ok") {
		t.Fatalf("show output missing validation status: %s", showOut)
	}

	sourcesOut, sourcesErr, err := executeRootCommand(t, "config", "sources")
	if err != nil {
		t.Fatalf("config sources failed: %v\nstderr: %s", err, sourcesErr)
	}
	if !strings.Contains(sourcesOut, "EFFECTIVE_SOURCE") {
		t.Fatalf("sources output missing effective source column: %s", sourcesOut)
	}
	if !strings.Contains(sourcesOut, "provider") || !strings.Contains(sourcesOut, "google") {
		t.Fatalf("sources output missing precedence values for provider: %s", sourcesOut)
	}
}

func TestConfigValidateReportsSchemaErrors(t *testing.T) {
	repoRoot := mustGetwd(t)
	homeDir := mustMkdirTempInRepo(t, repoRoot, "home-*")
	t.Setenv("HOME", homeDir)
	mustWriteFile(t, filepath.Join(homeDir, ".automergent", "config.yaml"), "mode: invalid\n")

	out, stderr, err := executeRootCommand(t, "config", "validate")
	if err == nil {
		t.Fatalf("expected validation failure, got success\nstdout: %s\nstderr: %s", out, stderr)
	}
	if !strings.Contains(out, "Validation: FAIL") {
		t.Fatalf("missing validation failure summary: %s", out)
	}
	if !strings.Contains(out, "mode") || !strings.Contains(out, "must be one of") {
		t.Fatalf("missing field-level schema error details: %s", out)
	}
}

func TestConfigValidateReportsLoadErrors(t *testing.T) {
	repoRoot := mustGetwd(t)
	homeDir := mustMkdirTempInRepo(t, repoRoot, "home-*")
	badConfigPath := filepath.Join(repoRoot, "test-invalid-config.yaml")
	t.Setenv("HOME", homeDir)
	mustWriteFile(t, badConfigPath, "provider: [\n")
	defer os.Remove(badConfigPath)

	_, _, err := executeRootCommand(t, "config", "validate", "--config", badConfigPath)
	if err == nil {
		t.Fatal("expected load failure for invalid yaml")
	}
	if !strings.Contains(err.Error(), "load configuration") {
		t.Fatalf("expected load configuration error, got: %v", err)
	}
}

func executeRootCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	originalCfgFile := cfgFile
	originalProfile := configProfile
	defer func() {
		cfgFile = originalCfgFile
		configProfile = originalProfile
	}()
	cfgFile = ""
	configProfile = ""

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs(args)

	_, err := rootCmd.ExecuteC()
	return stdout.String(), stderr.String(), err
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func mustMkdirTempInRepo(t *testing.T, dir string, pattern string) string {
	t.Helper()
	path, err := os.MkdirTemp(dir, pattern)
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })
	return path
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir all: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func mustChdir(t *testing.T, path string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		_ = os.Chdir(old)
	}
}
