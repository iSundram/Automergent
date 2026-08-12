package reasoning

import (
	"context"
	"testing"
)

func TestExtractIntent_TruncatesCleanly(t *testing.T) {
	got := extractIntent("This is a very long request that exceeds one hundred characters and should be truncated at exactly one hundred characters")
	if len(got) > 100 {
		t.Fatalf("intent too long: %d", len(got))
	}
}

func TestEngineProcessProducesPlan(t *testing.T) {
	e := NewEngine(nil)
	plan, err := e.Process(context.Background(), "Refactor entire project structure")
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if plan == nil || len(plan.Tasks) == 0 {
		t.Fatal("expected plan tasks")
	}
	if got := e.GetState(); got == nil || len(got.CompletedTasks) == 0 {
		t.Fatal("expected completed tasks in state")
	}
}

func TestPlanConvertsPlanningOutput(t *testing.T) {
	e := NewEngine(nil)
	analysis, err := e.Analyze(context.Background(), "Add a small helper function")
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	plan, err := e.Plan(context.Background(), analysis)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan == nil || len(plan.Tasks) == 0 {
		t.Fatal("expected tasks in execution plan")
	}
	if len(plan.Checkpoints) == 0 {
		t.Fatal("expected checkpoints in execution plan")
	}
}
