package tools

import "context"

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args map[string]any) (Result, error)
	RequiresConfirmation(mode string) bool
	IsConcurrencySafe(args map[string]any) bool
	IsReadOnly(args map[string]any) bool
	IsDestructive(args map[string]any) bool
	EstimatedCost() ToolCost
}

// Result holds the output of a tool execution.
type Result struct {
	Content  string
	Summary  string
	IsError  bool
	Metadata map[string]any
}

// BaseTool provides default implementations for concurrency safety methods.
// Tools can embed this to get safe defaults, then override specific methods as needed.
type BaseTool struct{}

// IsConcurrencySafe returns false by default (conservative approach).
func (b *BaseTool) IsConcurrencySafe(args map[string]any) bool {
	return false
}

// IsReadOnly returns false by default (conservative approach).
func (b *BaseTool) IsReadOnly(args map[string]any) bool {
	return false
}

// IsDestructive returns false by default.
func (b *BaseTool) IsDestructive(args map[string]any) bool {
	return false
}

// EstimatedCost returns a conservative default cost estimate.
func (b *BaseTool) EstimatedCost() ToolCost {
	return ToolCost{
		TokensApprox: 100,
		LatencyMs:    50,
		RiskLevel:    "low",
	}
}

// ToolCost estimates resource consumption and risk for a tool operation.
type ToolCost struct {
	TokensApprox int    // Estimated tokens consumed
	LatencyMs    int    // Typical latency in milliseconds
	RiskLevel    string // "low", "medium", "high"
}
