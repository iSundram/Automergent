package planning

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeRequest(t *testing.T) {
	p := NewPlanner(".")
	a := p.AnalyzeRequest("Fix internal/planning and add tests for replanning.")
	if a.RequestType != RequestTypeBugFix {
		t.Fatalf("expected bug fix, got %s", a.RequestType)
	}
	if len(a.Keywords) == 0 {
		t.Fatal("expected keywords")
	}
}

func TestGeneratePlanOrdersDependencies(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "alpha.go"), "package main\n// beta\n")
	mustWrite(t, filepath.Join(dir, "beta.go"), "package main\n")

	p := NewPlanner(dir)
	plan, err := p.GeneratePlan(context.Background(), "fix alpha.go and beta.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ExecutionOrder) == 0 {
		t.Fatal("expected execution order")
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(plan.Steps))
	}
}

func TestReplanIncrementsCount(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n")
	p := NewPlanner(dir)
	plan, err := p.GeneratePlan(context.Background(), "update main.go")
	if err != nil {
		t.Fatal(err)
	}
	replanned, err := p.Replan(context.Background(), plan, "needs tests too")
	if err != nil {
		t.Fatal(err)
	}
	if replanned.ReplanCount != plan.ReplanCount+1 {
		t.Fatalf("expected replan count %d, got %d", plan.ReplanCount+1, replanned.ReplanCount)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
