package shell

import (
	"context"
	"fmt"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// NewWaitTool creates a wait tool.
func NewWaitTool() *WaitTool { return &WaitTool{} }

// WaitTool pauses execution briefly — for polling background shells, waiting
// for servers to come up, or spacing out retries.
type WaitTool struct {
	tools.BaseTool
}

func (*WaitTool) Name() string        { return "wait" }
func (*WaitTool) Description() string { return "Wait for a short interval before continuing." }
func (*WaitTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"seconds": map[string]any{"type": "integer", "description": "How long to wait, 1-60 (default 5)"},
		},
	}
}
func (*WaitTool) RequiresConfirmation(string) bool      { return false }
func (*WaitTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*WaitTool) IsReadOnly(map[string]any) bool        { return true }
func (*WaitTool) IsDestructive(map[string]any) bool     { return false }
func (*WaitTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 10, LatencyMs: 5000, RiskLevel: "low"}
}
func (*WaitTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "shell",
		DisplayName: "Wait",
		InjectOrder: 90,
		WhenToUse:   "After starting an async shell you expect to finish soon, or before re-checking a server/log. Prefer checking output first — do not sleep on autopilot.",
	}
}

func (t *WaitTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	seconds := 5
	if v, ok := args["seconds"].(float64); ok {
		seconds = int(v)
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 60 {
		seconds = 60
	}
	select {
	case <-time.After(time.Duration(seconds) * time.Second):
		return tools.Result{Content: fmt.Sprintf("waited %ds", seconds), Summary: fmt.Sprintf("%ds", seconds)}, nil
	case <-ctx.Done():
		return tools.Result{IsError: true, Content: "wait cancelled"}, nil
	}
}
