package interaction

import (
	"context"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// NewFinishTool creates a finish tool.
func NewFinishTool() *FinishTool { return &FinishTool{} }

// FinishTool is the structured completion signal. Calling it ends the agent
// turn chain with the provided summary — the loop treats it as terminal.
type FinishTool struct {
	tools.BaseTool
}

func (*FinishTool) Name() string        { return "finish" }
func (*FinishTool) Description() string { return "End your turn: report completion (or the blocker) and stop calling tools." }
func (*FinishTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary":  map[string]any{"type": "string", "description": "What was accomplished, for the user-facing wrap-up"},
			"blocked":  map[string]any{"type": "boolean", "description": "True when stopping because work cannot proceed"},
			"evidence": map[string]any{"type": "string", "description": "Verification evidence: test/build output, files changed"},
		},
		"required": []string{"summary"},
	}
}
func (*FinishTool) RequiresConfirmation(string) bool      { return false }
func (*FinishTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*FinishTool) IsReadOnly(map[string]any) bool        { return true }
func (*FinishTool) IsDestructive(map[string]any) bool     { return false }
func (*FinishTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{TokensApprox: 20, LatencyMs: 10, RiskLevel: "low"} }
func (*FinishTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "interaction",
		DisplayName: "Finish",
		InjectOrder: 99,
		WhenToUse:   "ONLY after verifying your changes pass targeted tests/builds — include that evidence. Use blocked=true when something genuinely prevents progress.",
		WhenNotTo:   "Never call finish speculatively to end an uncomfortable turn; wanting to be done is not being done.",
	}
}

func (t *FinishTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	summary, _ := tools.StringArg(args, "summary")
	if strings.TrimSpace(summary) == "" {
		return tools.Result{IsError: true, Content: "finish requires `summary`"}, nil
	}
	blocked, _ := args["blocked"].(bool)
	evidence, _ := tools.StringArg(args, "evidence")

	content := summary
	if evidence != "" {
		content += "\n\nEvidence: " + evidence
	}
	status := "completed"
	if blocked {
		status = "blocked"
	}
	return tools.Result{
		Content:  content,
		Summary:  status,
		Metadata: map[string]any{"finish": true, "blocked": blocked},
	}, nil
}
