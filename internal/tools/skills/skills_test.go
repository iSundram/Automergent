package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedSkills(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "go-testing"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: go-testing\ndescription: How this repo runs Go tests\n---\nRun tests with `go test ./...`."
	if err := os.WriteFile(filepath.Join(dir, "go-testing", "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "release.md"), []byte("---\nname: release\ndescription: Cut a release\n---\nTag and push."), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscoverAndLoadSkill(t *testing.T) {
	dir := seedSkills(t)
	defer SetDirs()
	SetDirs(dir)

	disc := &DiscoverSkillsTool{}
	res, err := disc.Execute(context.Background(), map[string]any{})
	if err != nil || res.IsError {
		t.Fatalf("discover failed: %v %s", err, res.Content)
	}
	if !strings.Contains(res.Content, "go-testing") || !strings.Contains(res.Content, "release") {
		t.Fatalf("discover output missing skills:\n%s", res.Content)
	}

	// Query filters.
	res, _ = disc.Execute(context.Background(), map[string]any{"query": "release"})
	if !strings.Contains(res.Content, "release") || strings.Contains(res.Content, "go-testing") {
		t.Fatalf("query filter wrong:\n%s", res.Content)
	}

	// Loading returns the body.
	sk := &SkillTool{}
	res, err = sk.Execute(context.Background(), map[string]any{"name": "go-testing"})
	if err != nil || res.IsError {
		t.Fatalf("skill load failed: %v %s", err, res.Content)
	}
	if !strings.Contains(res.Content, "go test ./...") {
		t.Fatalf("skill body missing:\n%s", res.Content)
	}

	// Unknown skill errors.
	res, _ = sk.Execute(context.Background(), map[string]any{"name": "nope"})
	if !res.IsError {
		t.Fatal("expected error for unknown skill")
	}
}

func TestProjectSkillsOverrideUserSkills(t *testing.T) {
	user, proj := t.TempDir(), t.TempDir()
	write := func(dir, name, desc string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		md := "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody"
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(user, "shared", "user version")
	write(proj, "shared", "project version")
	defer SetDirs()
	SetDirs(user, proj)

	sk := &SkillTool{}
	res, _ := sk.Execute(context.Background(), map[string]any{"name": "shared"})
	if !strings.Contains(res.Content, proj) {
		t.Fatalf("project skill must win:\n%s", res.Content)
	}
}
