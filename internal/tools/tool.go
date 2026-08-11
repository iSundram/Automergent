package tools

import "context"

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args map[string]any) (Result, error)
	RequiresConfirmation(mode string) bool
}

// Result holds the output of a tool execution.
type Result struct {
	Content  string
	Summary  string
	IsError  bool
	Metadata map[string]any
}
