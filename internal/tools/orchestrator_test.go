package tools

import (
	"context"
	"strings"
	"testing"
)

type mockOrchestrationTool struct {
	BaseTool
	name         string
	readOnly     bool
	concurrency  bool
	confirmation bool
	risk         string
}

func (m *mockOrchestrationTool) Name() string        { return m.name }
func (m *mockOrchestrationTool) Description() string { return "mock tool" }
func (m *mockOrchestrationTool) Schema() map[string]any {
	return map[string]any{"type": "object"}
}
func (m *mockOrchestrationTool) Execute(_ context.Context, _ map[string]any) (Result, error) {
	return Result{Content: "ok"}, nil
}
func (m *mockOrchestrationTool) RequiresConfirmation(_ string) bool { return m.confirmation }
func (m *mockOrchestrationTool) IsConcurrencySafe(_ map[string]any) bool {
	return m.concurrency
}
func (m *mockOrchestrationTool) IsReadOnly(_ map[string]any) bool { return m.readOnly }
func (m *mockOrchestrationTool) EstimatedCost() ToolCost {
	cost := m.BaseTool.EstimatedCost()
	if m.risk != "" {
		cost.RiskLevel = m.risk
	}
	return cost
}

func TestOrchestratorExecute_RequestResponseFlow(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockOrchestrationTool{name: "read", readOnly: true, concurrency: true})
	reg.Register(&mockOrchestrationTool{name: "write", readOnly: false, concurrency: false})

	orchestrator := NewOrchestrator(reg.Get, func(_ context.Context, call OrchestrationCall) (Result, error) {
		return Result{Content: "ran:" + call.Name}, nil
	})

	resp := orchestrator.Execute(context.Background(), ExecutionRequest{
		Mode: "edit",
		Calls: []OrchestrationCall{
			{ID: "1", Name: "read"},
			{ID: "2", Name: "read"},
			{ID: "3", Name: "write"},
		},
		MaxParallelBatch: 10,
	})

	if len(resp.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(resp.Records))
	}

	if resp.Records[0].Strategy != ExecutionStrategyParallel || resp.Records[1].Strategy != ExecutionStrategyParallel {
		t.Fatalf("expected first two calls to be parallel, got %s and %s", resp.Records[0].Strategy, resp.Records[1].Strategy)
	}
	if resp.Records[2].Strategy != ExecutionStrategySequential {
		t.Fatalf("expected third call to be sequential, got %s", resp.Records[2].Strategy)
	}
	if len(resp.Records[2].Reasons) != 1 || resp.Records[2].Reasons[0] != ReasonCodeNotReadOnly {
		t.Fatalf("unexpected reason for write call: %#v", resp.Records[2].Reasons)
	}
	if resp.Records[0].Result.Content != "ran:read" || resp.Records[2].Result.Content != "ran:write" {
		t.Fatalf("unexpected results: %#v", resp.Records)
	}
}

func TestOrchestratorExecute_UnknownTool(t *testing.T) {
	reg := NewRegistry()
	orchestrator := NewOrchestrator(reg.Get, func(_ context.Context, _ OrchestrationCall) (Result, error) {
		return Result{}, nil
	})

	resp := orchestrator.Execute(context.Background(), ExecutionRequest{
		Calls: []OrchestrationCall{{ID: "x", Name: "missing"}},
	})

	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	if resp.Records[0].Strategy != ExecutionStrategySequential {
		t.Fatalf("expected sequential strategy, got %s", resp.Records[0].Strategy)
	}
	if len(resp.Records[0].Reasons) != 2 || resp.Records[0].Reasons[0] != ReasonCodeUnknownTool || resp.Records[0].Reasons[1] != ReasonCodeDeterministicFallback {
		t.Fatalf("unexpected reasons: %#v", resp.Records[0].Reasons)
	}
	if resp.Records[0].Fallback != ExecutionStrategySequential {
		t.Fatalf("expected deterministic fallback to sequential, got %s", resp.Records[0].Fallback)
	}
}

func TestOrchestratorExecute_ExecutorErrorBecomesErrorResult(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockOrchestrationTool{name: "read", readOnly: true, concurrency: true})

	orchestrator := NewOrchestrator(reg.Get, func(_ context.Context, _ OrchestrationCall) (Result, error) {
		return Result{}, context.DeadlineExceeded
	})

	resp := orchestrator.Execute(context.Background(), ExecutionRequest{
		Calls: []OrchestrationCall{{ID: "e", Name: "read"}},
	})

	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	if !resp.Records[0].Result.IsError {
		t.Fatal("expected error result")
	}
	if !strings.Contains(resp.Records[0].Result.Content, "deadline exceeded") {
		t.Fatalf("expected error content, got %q", resp.Records[0].Result.Content)
	}
	if resp.Records[0].Error == "" {
		t.Fatal("expected record error to be populated")
	}
}

func TestOrchestratorExecute_MaxParallelBatchLimit(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockOrchestrationTool{name: "read", readOnly: true, concurrency: true})

	orchestrator := NewOrchestrator(reg.Get, func(_ context.Context, call OrchestrationCall) (Result, error) {
		return Result{Content: call.ID}, nil
	})

	resp := orchestrator.Execute(context.Background(), ExecutionRequest{
		MaxParallelBatch: 2,
		Calls: []OrchestrationCall{
			{ID: "1", Name: "read"},
			{ID: "2", Name: "read"},
			{ID: "3", Name: "read"},
		},
	})

	if len(resp.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(resp.Records))
	}
	if resp.Records[0].BatchIndex != 0 || resp.Records[1].BatchIndex != 0 || resp.Records[2].BatchIndex != 1 {
		t.Fatalf("expected batch split at 2 calls, got indexes: %d, %d, %d",
			resp.Records[0].BatchIndex, resp.Records[1].BatchIndex, resp.Records[2].BatchIndex)
	}
}

func TestOrchestratorExecute_UnknownRiskUsesDeterministicFallback(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockOrchestrationTool{name: "read", readOnly: true, concurrency: true, risk: "experimental"})

	orchestrator := NewOrchestrator(reg.Get, func(_ context.Context, call OrchestrationCall) (Result, error) {
		return Result{Content: call.ID}, nil
	})

	resp := orchestrator.Execute(context.Background(), ExecutionRequest{
		Calls: []OrchestrationCall{{ID: "r1", Name: "read"}},
	})

	if len(resp.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.Records))
	}
	record := resp.Records[0]
	if record.Strategy != ExecutionStrategySequential {
		t.Fatalf("expected sequential strategy, got %s", record.Strategy)
	}
	if record.Fallback != ExecutionStrategySequential {
		t.Fatalf("expected deterministic fallback target to be sequential, got %s", record.Fallback)
	}
	if len(record.Reasons) != 2 || record.Reasons[0] != ReasonCodeUnknownRisk || record.Reasons[1] != ReasonCodeDeterministicFallback {
		t.Fatalf("unexpected reasons: %#v", record.Reasons)
	}
}
