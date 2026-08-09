package sdk

import (
	"context"

	"github.com/iSundram/Automergent/internal/tools"
)

// Tool is the public SDK interface for custom tools.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, args map[string]any) (Result, error)
	RequiresConfirmation(mode string) bool
}

// Result is the public SDK result type.
type Result = tools.Result

// Register registers a custom tool with the global registry.
func Register(t Tool) {
	tools.Register(&sdkAdapter{t})
}

type sdkAdapter struct{ t Tool }

func (a *sdkAdapter) Name() string                          { return a.t.Name() }
func (a *sdkAdapter) Description() string                   { return a.t.Description() }
func (a *sdkAdapter) Schema() map[string]any                { return a.t.Schema() }
func (a *sdkAdapter) RequiresConfirmation(mode string) bool { return a.t.RequiresConfirmation(mode) }
func (a *sdkAdapter) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	return a.t.Execute(ctx, args)
}
