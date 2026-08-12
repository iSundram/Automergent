package planning

import "github.com/iSundram/Automergent/internal/tools"

func (t *Tool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *Tool) IsReadOnly(args map[string]any) bool        { return true }
func (t *Tool) IsDestructive(args map[string]any) bool     { return false }
func (t *Tool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 1000, RiskLevel: "low"}
}

func (t *ReplanTool) IsConcurrencySafe(args map[string]any) bool { return true }
func (t *ReplanTool) IsReadOnly(args map[string]any) bool        { return true }
func (t *ReplanTool) IsDestructive(args map[string]any) bool     { return false }
func (t *ReplanTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 1000, RiskLevel: "low"}
}
