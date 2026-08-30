package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/components"
)

func TestIsArtifactWrite(t *testing.T) {
	cases := []struct {
		tool, path string
		want       bool
	}{
		{"write_file", ".automergent/artifacts/plan.md", true},
		{"write_file", "notes/demo_artifact.md", true},
		{"write_file", "docs/review.md", true},
		{"write_file", "README.md", false},                    // conventional doc
		{"write_file", "internal/agent/title.go", false},      // code file
		{"write_file", ".env", false},                         // dotfile, no doc ext
		{"write_file", "", false},                             // no path
		{"edit_file", ".automergent/artifacts/plan.md", false},      // edits don't register
		{"write_file", "notes/CLAUDE.md", false},              // noise doc
	}
	for _, c := range cases {
		if got := isArtifactWrite(c.tool, c.path); got != c.want {
			t.Errorf("isArtifactWrite(%q, %q) = %v, want %v", c.tool, c.path, got, c.want)
		}
	}
}

func TestArtifactKind(t *testing.T) {
	cases := map[string]string{
		".automergent/artifacts/plan.md":        "plan",
		"notes/design-spec.md":            "design",
		"docs/code-review.md":             "review",
		"reports/summary.md":              "summary",
		"notes/demo_artifact.md":          "document",
	}
	for path, want := range cases {
		if got := artifactKind(path); got != want {
			t.Errorf("artifactKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRegisterArtifactAndReviewFlow(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, "demo_artifact.md")
	if err := os.WriteFile(path, []byte("# Demo\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.registerArtifact(path)
	if len(app.artifacts) != 1 || app.artifacts[0].Kind != "document" {
		t.Fatalf("artifact not registered: %+v", app.artifacts)
	}
	if app.pendingArtifactCount() != 1 {
		t.Fatal("expected 1 pending artifact")
	}

	// The status bar surfaces the pending count.
	app.statusBar.SetWidth(120)
	if !strings.Contains(app.statusBar.View(), "1 artifact · /artifact to review") {
		t.Fatalf("status bar missing artifact segment:\n%s", app.statusBar.View())
	}

	// /artifact opens the review browser with the row.
	app.handleSlashCommand("/artifact")
	if !app.artifactBrowser.Visible() {
		t.Fatal("expected artifact browser visible")
	}
	app.artifactBrowser.SetSize(90, 24)
	view := ansiStrip(app.artifactBrowser.View())
	for _, want := range []string{"demo_artifact.md", "Action required (1 left)", "y approve"} {
		if !strings.Contains(view, want) {
			t.Fatalf("browser view missing %q:\n%s", want, view)
		}
	}

	// y approves; the pending count clears.
	_, cmd := app.artifactBrowser.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("expected decision message")
	}
	app.Update(cmd())
	if app.pendingArtifactCount() != 0 {
		t.Fatal("expected no pending artifacts after approve")
	}
	if app.artifacts[0].Status != components.ArtifactApproved {
		t.Fatalf("expected approved status, got %v", app.artifacts[0].Status)
	}
	if !strings.Contains(ansiStrip(app.artifactBrowser.View()), "All artifacts reviewed") {
		t.Fatalf("expected all-reviewed state:\n%s", app.artifactBrowser.View())
	}
}

func TestArtifactPreviewMode(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Plan\nstep one\nstep two\nstep three\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.registerArtifact(path)
	app.handleSlashCommand("/artifact")
	app.artifactBrowser.SetSize(90, 24)

	// p opens the preview with line numbers.
	ab, _ := app.artifactBrowser.Update(tea.KeyPressMsg{Code: 'p'})
	if !ab.Previewing() {
		t.Fatal("expected preview mode")
	}
	view := ansiStrip(ab.View())
	for _, want := range []string{"plan.md", "1", "# Plan", "step three"} {
		if !strings.Contains(view, want) {
			t.Fatalf("preview missing %q:\n%s", want, view)
		}
	}

	// / starts search; typing and enter jumps to the match.
	ab, _ = ab.Update(tea.KeyPressMsg{Code: '/'})
	for _, ch := range "three" {
		ab, _ = ab.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	ab, _ = ab.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(ansiStrip(ab.View()), "step three") {
		t.Fatalf("search jump failed:\n%s", ab.View())
	}

	// esc returns to the list.
	ab, _ = ab.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if ab.Previewing() {
		t.Fatal("expected to return to list mode")
	}
}

func TestArtifactApproveAllAndReject(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	var paths []string
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		app.registerArtifact(p)
		paths = append(paths, p)
	}
	if app.pendingArtifactCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", app.pendingArtifactCount())
	}

	// Reject the first, approve-all the rest.
	app.applyArtifactDecision(paths[0], false)
	if app.artifacts[0].Status != components.ArtifactRejected {
		t.Fatal("expected rejected")
	}
	app.applyArtifactApproveAll()
	if app.pendingArtifactCount() != 0 {
		t.Fatal("expected all decided after approve-all")
	}
	for i := 1; i < 3; i++ {
		if app.artifacts[i].Status != components.ArtifactApproved {
			t.Fatalf("artifact %d not approved", i)
		}
	}
}

func TestMaybeRegisterArtifactFromToolDone(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, ".automergent", "artifacts", "plan.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.maybeRegisterArtifact(agent.ToolDoneEvent{
		Name:    "write_file",
		Context: path,
	})
	if len(app.artifacts) != 1 {
		t.Fatalf("expected artifact registered from tool event, got %d", len(app.artifacts))
	}

	// A non-doc write must not register.
	app.maybeRegisterArtifact(agent.ToolDoneEvent{
		Name:    "write_file",
		Context: filepath.Join(dir, "main.go"),
	})
	if len(app.artifacts) != 1 {
		t.Fatalf("code write must not register, got %d", len(app.artifacts))
	}
}

func TestSeedArtifactsFromDisk(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	artDir := filepath.Join(dir, ".automergent", "artifacts")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artDir, "plan.md"), []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.seedArtifactsFromDisk()
	if len(app.artifacts) != 1 {
		t.Fatalf("expected seeded artifact, got %d", len(app.artifacts))
	}
	if app.artifacts[0].Kind != "plan" {
		t.Fatalf("expected plan kind, got %q", app.artifacts[0].Kind)
	}
}

func TestArtifactBrowserKeysDoNotLeakToInput(t *testing.T) {
	app := newTestApp(t)
	app.handleSlashCommand("/artifact")
	app.artifactBrowser.SetSize(90, 24)
	for _, ch := range "ynp" {
		app.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
	if got := app.input.Value(); got != "" {
		t.Fatalf("keys leaked into input: %q", got)
	}
}
