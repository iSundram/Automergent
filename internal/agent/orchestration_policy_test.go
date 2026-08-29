package agent

import (
	"context"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

type policyTestTool struct {
	name            string
	readOnly        bool
	concurrencySafe bool
	destructive     bool
	requiresConfirm bool
	riskLevel       string
}

func (t policyTestTool) Name() string           { return t.name }
func (t policyTestTool) Description() string    { return t.name }
func (t policyTestTool) Schema() map[string]any { return map[string]any{} }
func (t policyTestTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (t policyTestTool) RequiresConfirmation(string) bool      { return t.requiresConfirm }
func (t policyTestTool) IsConcurrencySafe(map[string]any) bool { return t.concurrencySafe }
func (t policyTestTool) IsReadOnly(map[string]any) bool        { return t.readOnly }
func (t policyTestTool) IsDestructive(map[string]any) bool     { return t.destructive }
func (t policyTestTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{RiskLevel: t.riskLevel}
}

func newPolicyTestAgent(reg *tools.Registry) *Agent {
	return &Agent{
		// Mode "auto" so executeTool reaches the actual tool without waiting on
		// a confirmation channel nobody serves in these tests. "edit" used to
		// mean "the tool decides", but it now canonicalises to "manual", under
		// which the write tool blocks for up to ConfirmationTimeout — which is
		// why this suite used to hang for ten minutes before failing.
		cfg:                 &config.Config{Mode: "auto"},
		tools:               reg,
		events:              make(chan Event, 64),
		sessionAllowedTools: map[string]bool{},
	}
}

func TestExecuteToolCallsParallelSelectsPolicyStrategy(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(policyTestTool{name: "read_safe", readOnly: true, concurrencySafe: true, riskLevel: "low"})
	reg.Register(policyTestTool{name: "write_unsafe", readOnly: false, concurrencySafe: false, riskLevel: "medium"})
	ag := newPolicyTestAgent(reg)

	results := ag.executeToolCallsParallel(context.Background(), []ai.ToolCall{
		{ID: "tc1", Name: "read_safe"},
		{ID: "tc2", Name: "read_safe"},
		{ID: "tc3", Name: "write_unsafe"},
	})
	if len(results) != 3 {
		t.Fatalf("expected 3 executed calls, got %d", len(results))
	}
	if results[0].decision.Strategy != SchedulingStrategyParallel || results[1].decision.Strategy != SchedulingStrategyParallel {
		t.Fatalf("expected first two calls to be parallel eligible")
	}
	if results[2].decision.Strategy != SchedulingStrategySequential {
		t.Fatalf("expected write call to be sequential")
	}

	records := ag.DecisionRecords()
	if len(records) != 3 {
		t.Fatalf("expected 3 decision records, got %d", len(records))
	}
	if records[0].Strategy != SchedulingStrategyParallel || records[0].Reasons[0].Code != PolicyReasonParallelEligible {
		t.Fatalf("expected first decision to be parallel with explainable reason, got %+v", records[0])
	}
	if records[2].Strategy != SchedulingStrategySequential || !hasReasonCode(records[2].Reasons, PolicyReasonWriteOperation) {
		t.Fatalf("expected write tool to be sequential with write reason, got %+v", records[2])
	}
}

func TestEvaluateToolDecisionUsesDeterministicFallbackForUnknownRisk(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(policyTestTool{name: "read_safe", readOnly: true, concurrencySafe: true, riskLevel: "experimental"})
	ag := newPolicyTestAgent(reg)

	decision := ag.evaluateToolDecision(ai.ToolCall{ID: "tc1", Name: "read_safe"})
	if decision.Strategy != SchedulingStrategySequential {
		t.Fatalf("expected sequential strategy, got %s", decision.Strategy)
	}
	if !decision.DeterministicFallback {
		t.Fatalf("expected deterministic fallback to be true")
	}
	if !hasReasonCode(decision.Reasons, PolicyReasonUnknownRisk) || !hasReasonCode(decision.Reasons, PolicyReasonDeterministicFallback) {
		t.Fatalf("expected unknown risk and fallback reasons, got %+v", decision.Reasons)
	}
}

func TestToolCallEventIncludesDecisionReasons(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(policyTestTool{name: "read_safe", readOnly: true, concurrencySafe: true, riskLevel: "low"})
	ag := newPolicyTestAgent(reg)

	ag.executeToolCallsParallel(context.Background(), []ai.ToolCall{{ID: "tc1", Name: "read_safe"}})

	found := false
	for {
		select {
		case ev := <-ag.events:
			if ev.Type != EventToolCall {
				continue
			}
			te, ok := ev.Payload.(ToolCallEvent)
			if !ok {
				t.Fatalf("expected ToolCallEvent payload, got %T", ev.Payload)
			}
			if te.Decision.Strategy != SchedulingStrategyParallel || !hasReasonCode(te.Decision.Reasons, PolicyReasonParallelEligible) {
				t.Fatalf("expected policy decision and reasons in event, got %+v", te.Decision)
			}
			found = true
		default:
			if !found {
				t.Fatalf("expected tool call event")
			}
			return
		}
	}
}
