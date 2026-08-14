package agent

import (
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

type SchedulingStrategy string

const (
	SchedulingStrategyParallel   SchedulingStrategy = "parallel"
	SchedulingStrategySequential SchedulingStrategy = "sequential"
)

type PolicyReasonCode string

const (
	PolicyReasonParallelEligible      PolicyReasonCode = "parallel_eligible"
	PolicyReasonUnknownTool           PolicyReasonCode = "unknown_tool"
	PolicyReasonWriteOperation        PolicyReasonCode = "write_operation"
	PolicyReasonNotConcurrencySafe    PolicyReasonCode = "not_concurrency_safe"
	PolicyReasonRequiresConfirmation  PolicyReasonCode = "requires_confirmation"
	PolicyReasonDestructiveOperation  PolicyReasonCode = "destructive_operation"
	PolicyReasonHighRisk              PolicyReasonCode = "high_risk"
	PolicyReasonUnknownRisk           PolicyReasonCode = "unknown_risk"
	PolicyReasonDeterministicFallback PolicyReasonCode = "deterministic_fallback"
)

type PolicyReason struct {
	Code    PolicyReasonCode `json:"code"`
	Message string           `json:"message"`
}

type ToolDecisionRecord struct {
	ToolCallID             string             `json:"toolCallId"`
	ToolName               string             `json:"toolName"`
	Strategy               SchedulingStrategy `json:"strategy"`
	Reasons                []PolicyReason     `json:"reasons"`
	DeterministicFallback  bool               `json:"deterministicFallback"`
	FallbackTargetStrategy SchedulingStrategy `json:"fallbackTargetStrategy,omitempty"`
}

func (a *Agent) evaluateToolDecision(tc ai.ToolCall) ToolDecisionRecord {
	decision := ToolDecisionRecord{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
	}

	tool, ok := a.tools.Get(tc.Name)
	if !ok {
		decision.Strategy = SchedulingStrategySequential
		decision.DeterministicFallback = true
		decision.FallbackTargetStrategy = SchedulingStrategySequential
		decision.Reasons = []PolicyReason{
			{Code: PolicyReasonUnknownTool, Message: "tool is not registered"},
			{Code: PolicyReasonDeterministicFallback, Message: "falling back to sequential execution"},
		}
		return decision
	}

	blockingReasons := make([]PolicyReason, 0, 5)
	mode := ""
	if a.cfg != nil {
		mode = a.cfg.Mode
	}
	if !tool.IsReadOnly(tc.Args) {
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonWriteOperation,
			Message: "tool is not read-only",
		})
	}
	if !tool.IsConcurrencySafe(tc.Args) {
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonNotConcurrencySafe,
			Message: "tool is not concurrency-safe",
		})
	}
	if tool.RequiresConfirmation(mode) {
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonRequiresConfirmation,
			Message: "tool requires confirmation in current mode",
		})
	}
	if tool.IsDestructive(tc.Args) {
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonDestructiveOperation,
			Message: "tool is destructive",
		})
	}

	risk := strings.ToLower(strings.TrimSpace(tool.EstimatedCost().RiskLevel))
	switch risk {
	case "high":
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonHighRisk,
			Message: "tool has high estimated risk",
		})
	case "", "low", "medium":
	default:
		blockingReasons = append(blockingReasons, PolicyReason{
			Code:    PolicyReasonUnknownRisk,
			Message: "tool risk level is unknown",
		})
	}

	if len(blockingReasons) == 0 {
		decision.Strategy = SchedulingStrategyParallel
		decision.Reasons = []PolicyReason{
			{Code: PolicyReasonParallelEligible, Message: "tool meets parallel safety policy"},
		}
		return decision
	}

	decision.Strategy = SchedulingStrategySequential
	decision.Reasons = blockingReasons
	if hasReasonCode(blockingReasons, PolicyReasonUnknownRisk) {
		decision.DeterministicFallback = true
		decision.FallbackTargetStrategy = SchedulingStrategySequential
		decision.Reasons = append(decision.Reasons, PolicyReason{
			Code:    PolicyReasonDeterministicFallback,
			Message: "using deterministic sequential fallback for unknown risk",
		})
	}

	return decision
}

func hasReasonCode(reasons []PolicyReason, code PolicyReasonCode) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func (a *Agent) storeDecisionRecords(records []ToolDecisionRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.decisionRecords = append([]ToolDecisionRecord(nil), records...)
}

func (a *Agent) DecisionRecords() []ToolDecisionRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]ToolDecisionRecord(nil), a.decisionRecords...)
}
