package interaction

import "github.com/iSundram/Automergent/internal/tools"

func (t *AskUserTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *AskUserTool) IsReadOnly(args map[string]any) bool        { return false }
func (t *AskUserTool) IsDestructive(args map[string]any) bool     { return false }
func (t *AskUserTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 50, LatencyMs: 5000, RiskLevel: "low"}
}

func (t *NotifyTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *NotifyTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *NotifyTool) IsDestructive(args map[string]any) bool     { return false }
func (t *NotifyTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 10, LatencyMs: 10, RiskLevel: "low"}
}
