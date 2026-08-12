package sdk

import (
	"context"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/pkg/sdk/builder"
	"github.com/iSundram/Automergent/pkg/sdk/schema"
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
func (a *sdkAdapter) IsConcurrencySafe(args map[string]any) bool { return false }
func (a *sdkAdapter) IsReadOnly(args map[string]any) bool        { return false }
func (a *sdkAdapter) IsDestructive(args map[string]any) bool     { return false }
func (a *sdkAdapter) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 100, LatencyMs: 100, RiskLevel: "low"}
}

// ============================================================================
// Fluent API - Re-exported for convenience
// ============================================================================

// NewTool creates a new tool builder with the given name.
// This is the entry point for the fluent API.
//
// Example:
//
//	tool := sdk.NewTool("read_file").
//	    Description("Read file content").
//	    Param("path", sdk.String().Required().Description("File path")).
//	    Param("encoding", sdk.String().Default("utf-8")).
//	    Execute(func(ctx context.Context, args *sdk.Args) (sdk.Result, error) {
//	        path := args.String("path")
//	        // ... implementation
//	        return sdk.Result{Content: content}, nil
//	    }).
//	    Build()
func NewTool(name string) *builder.ToolBuilder {
	return builder.NewTool(name)
}

// NewTypedTool creates a new typed tool builder.
// This allows for type-safe argument handling using struct binding.
//
// Example:
//
//	type ReadFileArgs struct {
//	    Path     string `arg:"path"`
//	    Encoding string `arg:"encoding"`
//	}
//
//	tool := sdk.NewTypedTool[ReadFileArgs]("read_file").
//	    Description("Read file content").
//	    Param("path", sdk.String().Required()).
//	    Execute(func(ctx context.Context, args ReadFileArgs) (sdk.Result, error) {
//	        // Use args.Path directly
//	        return sdk.Result{Content: content}, nil
//	    }).
//	    Build()
func NewTypedTool[T any](name string) *builder.TypedToolBuilder[T] {
	return builder.NewTypedTool[T](name)
}

// ============================================================================
// Schema Builders - Re-exported from schema package
// ============================================================================

// Args provides type-safe argument binding and extraction.
type Args = schema.Args

// Builder provides a fluent API for constructing parameter schemas.
type Builder = schema.Builder

// String creates a new string parameter schema builder.
func String() *schema.Builder {
	return schema.String()
}

// Number creates a new number parameter schema builder.
func Number() *schema.Builder {
	return schema.Number()
}

// Integer creates a new integer parameter schema builder.
func Integer() *schema.Builder {
	return schema.Integer()
}

// Boolean creates a new boolean parameter schema builder.
func Boolean() *schema.Builder {
	return schema.Boolean()
}

// Array creates a new array parameter schema builder.
func Array() *schema.Builder {
	return schema.Array()
}

// Object creates a new object parameter schema builder.
func Object() *schema.Builder {
	return schema.Object()
}

// ============================================================================
// FluentTool - Re-exported from builder package
// ============================================================================

// FluentTool is a tool built using the fluent API.
type FluentTool = builder.FluentTool

// Example represents a usage example for documentation.
type Example = builder.Example

// Handler is the function signature for tool execution.
type Handler = builder.Handler
