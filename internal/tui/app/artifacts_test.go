package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tools"
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
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Demo plan\nline two\nline three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.registerArtifact(path)
	if len(app.artifacts) != 1 || app.artifacts[0].Kind != "plan" {
		t.Fatalf("artifact not registered: %+v", app.artifacts)
	}
	if app.pendingArtifactCount() != 1 {
		t.Fatal("expected 1 pending plan")
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
	for _, want := range []string{"plan.md", "Action required (1 left)", "y approve"} {
		if !strings.Contains(view, want) {
			t.Fatalf("browser view missing %q:\n%s", want, view)
		}
	}

	// y approves; the pending count clears. The agent is idle here, so the
	// approval also puts it to work on the plan.
	_, cmd := app.artifactBrowser.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("expected decision message")
	}
	app.Update(cmd())
	if app.pendingArtifactCount() != 0 {
		t.Fatal("expected no pending plans after approve")
	}
	if app.artifacts[0].Status != components.ArtifactApproved {
		t.Fatalf("expected approved status, got %v", app.artifacts[0].Status)
	}
}

func TestNonPlanArtifactHasNoDecision(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, "demo_artifact.md")
	if err := os.WriteFile(path, []byte("# Demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.registerArtifact(path)
	if app.artifacts[0].Kind != "document" {
		t.Fatalf("expected document kind, got %q", app.artifacts[0].Kind)
	}

	// No pending count: only plans need decisions.
	if app.pendingArtifactCount() != 0 {
		t.Fatal("documents must not count as pending")
	}

	// y on a non-plan does nothing.
	app.handleSlashCommand("/artifact")
	app.artifactBrowser.SetSize(90, 24)
	_, cmd := app.artifactBrowser.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd != nil {
		t.Fatal("y on a non-plan must not emit a decision")
	}

	// A direct decision call is refused too.
	app.applyArtifactDecision(path, true, "")
	if app.artifacts[0].Status != components.ArtifactPending {
		t.Fatal("non-plan artifacts must not be approvable")
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

func TestArtifactApproveAllRejectAndReason(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	var paths []string
	for _, name := range []string{"plan-a.md", "plan-b.md", "plan-c.md"} {
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

	// A reject without a reason never lands.
	app.applyArtifactDecision(paths[0], false, "")
	if app.artifacts[0].Status != components.ArtifactPending {
		t.Fatal("reasonless reject must be refused")
	}
	// With a reason it does, and the reason is kept as a comment.
	app.applyArtifactDecision(paths[0], false, "add rollback steps")
	if app.artifacts[0].Status != components.ArtifactRejected {
		t.Fatal("expected rejected")
	}
	if len(app.artifacts[0].Comments) != 1 || !strings.Contains(app.artifacts[0].Comments[0], "add rollback steps") {
		t.Fatalf("rejection reason not kept: %+v", app.artifacts[0].Comments)
	}

	// Approve-all clears the rest.
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

func TestArtifactComment(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.registerArtifact(path)

	app.applyArtifactComment(path, "please reconsider step 3")
	if len(app.artifacts[0].Comments) != 1 {
		t.Fatalf("comment not stored: %+v", app.artifacts[0].Comments)
	}
	// The comment rides into the persisted registry.
	app.persistArtifacts()
	if !strings.Contains(app.sess.Metadata["artifacts"], "please reconsider step 3") {
		t.Fatalf("comment not persisted: %s", app.sess.Metadata["artifacts"])
	}
}

func TestArtifactsAreSessionScoped(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.registerArtifact(path)
	if len(app.artifacts) != 1 {
		t.Fatal("setup: artifact not registered")
	}

	// /new clears the registry: a fresh session shows nothing.
	app.handleSlashCommand("/new")
	if len(app.artifacts) != 0 {
		t.Fatalf("new session must start artifact-free, got %d", len(app.artifacts))
	}

	// A resumed session restores exactly its own artifacts from metadata.
	app.sess.Metadata["artifacts"] = `[{"path":"` + path + `","kind":"plan","status":2}]`
	app.loadArtifactsForSession()
	if len(app.artifacts) != 1 {
		t.Fatalf("resume must restore this session's artifacts, got %d", len(app.artifacts))
	}
	if app.artifacts[0].Status != components.ArtifactRejected {
		t.Fatalf("status must survive the round-trip, got %v", app.artifacts[0].Status)
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

func TestArtifactToolRegistersWithMetadata(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	path := filepath.Join(dir, ".automergent", "artifacts", "plan.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Migration plan\n\nStep one.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.maybeRegisterArtifact(agent.ToolDoneEvent{
		Name:    "artifact",
		Context: path,
		Result: tools.Result{
			Summary: "artifact plan.md (2 lines)",
			Metadata: map[string]any{
				"artifact_title":   "Migration plan",
				"artifact_kind":    "plan",
				"request_feedback": true,
			},
		},
	})

	if len(app.artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(app.artifacts))
	}
	if app.artifacts[0].Title != "Migration plan" || app.artifacts[0].Kind != "plan" {
		t.Fatalf("metadata not applied: %+v", app.artifacts[0])
	}
	if !strings.Contains(app.statusBar.View(), "Artifact review requested") {
		if app.statusBar.Status() != "Artifact review requested — /artifact" {
			t.Fatalf("review request not surfaced: %q", app.statusBar.Status())
		}
	}
}

func TestPlanReviewFlowResolvesExitPlanMode(t *testing.T) {
	app := newTestApp(t)
	app.cfg.Mode = "manual"
	app.planModePrev = "manual"
	dir := t.TempDir()
	app.workDir = dir
	planPath := filepath.Join(dir, ".automergent", "artifacts", "plan.md")

	// Simulate the tool-side reviewer wiring: begin a review and resolve it.
	pr := &pendingPlanReview{
		planPath: planPath,
		summary:  "Refactor the parser",
		reply:    make(chan planDecision, 1),
		done:     make(chan struct{}),
	}
	app.Update(planReviewRequestedMsg{pr})
	if !app.artifactBrowser.Visible() {
		t.Fatal("plan review must open the artifact browser")
	}
	if app.pendingPlanReview != pr {
		t.Fatal("review must be parked on the app")
	}

	// The user approves via the artifact browser (y).
	_, cmd := app.artifactBrowser.Update(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("expected decision message")
	}
	app.Update(cmd())

	select {
	case d := <-pr.reply:
		if !d.approved {
			t.Fatal("approval must reach the waiting tool")
		}
	default:
		t.Fatal("tool was not answered")
	}
	if app.pendingPlanReview != nil {
		t.Fatal("review must be cleared after the decision")
	}
	// Plan mode is not active here, so the mode stays untouched.
	if app.cfg.Mode != "manual" {
		t.Fatalf("mode unexpectedly changed: %q", app.cfg.Mode)
	}
}

func TestPlanApprovalExitsPlanMode(t *testing.T) {
	app := newTestApp(t)
	app.cfg.Mode = "plan"
	app.planModePrev = "accept-edits"
	dir := t.TempDir()
	app.workDir = dir
	planPath := filepath.Join(dir, ".automergent", "artifacts", "plan.md")

	pr := &pendingPlanReview{
		planPath: planPath,
		summary:  "Add feature",
		reply:    make(chan planDecision, 1),
		done:     make(chan struct{}),
	}
	app.Update(planReviewRequestedMsg{pr})
	app.resolvePlanReview(true, "")

	if app.cfg.Mode != "accept-edits" {
		t.Fatalf("approval must restore the previous mode, got %q", app.cfg.Mode)
	}
	select {
	case d := <-pr.reply:
		if !d.approved {
			t.Fatal("expected approval delivered")
		}
	default:
		t.Fatal("tool was not answered")
	}
}

func TestCancelPlanReviewOnBrowserClose(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	app.workDir = dir
	pr := &pendingPlanReview{
		planPath: filepath.Join(dir, "plan.md"),
		summary:  "Whatever",
		reply:    make(chan planDecision, 1),
		done:     make(chan struct{}),
	}
	app.Update(planReviewRequestedMsg{pr})

	// esc closes the browser without a decision; the waiting tool must be
	// released, not left blocked.
	app.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if app.pendingPlanReview != nil {
		t.Fatal("review must be cancelled when the browser closes")
	}
	select {
	case <-pr.done:
	default:
		t.Fatal("tool goroutine still blocked after cancel")
	}
}
