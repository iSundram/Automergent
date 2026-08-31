package planmode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnterPlanModeSwitchesMode(t *testing.T) {
	defer SetModeChanger(nil)
	var got string
	SetModeChanger(func(mode string) error {
		got = mode
		return nil
	})
	tool := &EnterPlanModeTool{}
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil || res.IsError {
		t.Fatalf("enter failed: %v %s", err, res.Content)
	}
	if got != "plan" {
		t.Fatalf("mode changer not called with plan, got %q", got)
	}
}

func TestExitPlanModeBlocksUntilDecision(t *testing.T) {
	defer SetPlanReviewer(nil)
	decided := make(chan struct{})
	SetPlanReviewer(func(ctx context.Context, planPath, plan string) (bool, string, error) {
		<-decided
		return true, "", nil
	})
	tool := &ExitPlanModeTool{}

	type outcome struct {
		res interface {
			GetContent() string
		}
		raw *string
	}
	_ = outcome{}

	done := make(chan string, 1)
	go func() {
		res, err := tool.Execute(context.Background(), map[string]any{"plan": "do the thing"})
		if err != nil {
			done <- "err:" + err.Error()
			return
		}
		done <- res.Content
	}()

	select {
	case <-done:
		t.Fatal("exit_plan_mode must block until the reviewer answers")
	case <-time.After(50 * time.Millisecond):
	}
	close(decided)
	out := <-done
	if !strings.Contains(out, "approved") {
		t.Fatalf("expected approval result, got: %s", out)
	}
}

func TestExitPlanModeRejectionCarriesFeedback(t *testing.T) {
	defer SetPlanReviewer(nil)
	SetPlanReviewer(func(ctx context.Context, planPath, plan string) (bool, string, error) {
		return false, "add rollback steps", nil
	})
	tool := &ExitPlanModeTool{}
	res, err := tool.Execute(context.Background(), map[string]any{"plan": "do it"})
	if err != nil || res.IsError {
		t.Fatalf("execute failed: %v %s", err, res.Content)
	}
	if !strings.Contains(res.Content, "add rollback steps") {
		t.Fatalf("feedback missing:\n%s", res.Content)
	}
}

func TestVerifyPlanExecution(t *testing.T) {
	// A git repo with a plan artifact naming one changed and one unchanged file.
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-qm", "init")
	// Modify a.go after commit so it shows as changed.
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a // changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	planPath := filepath.Join(dir, "plan.md")
	plan := "# Plan\n\nChanges:\n- `a.go` — update\n- `b.go` — no-op\n"
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := &VerifyPlanExecutionTool{}
	res, err := tool.Execute(context.Background(), map[string]any{"plan_path": planPath})
	if err != nil || res.IsError {
		t.Fatalf("verify failed: %v %s", err, res.Content)
	}
	if !strings.Contains(res.Content, "✓ a.go") {
		t.Fatalf("changed file not verified:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "! b.go") || strings.Contains(res.Content, "✗ b.go") {
		t.Fatalf("unchanged file should warn, not fail hard:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "1/2 plan files verified") {
		t.Fatalf("verdict missing:\n%s", res.Content)
	}
}
