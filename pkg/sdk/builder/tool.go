// Package builder provides a fluent API for creating tools.
package builder

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/pkg/sdk/schema"
)

// Handler is the function signature for tool execution.
type Handler func(ctx context.Context, args *schema.Args) (Result, error)

// GenericHandler is a typed handler using struct binding.
type GenericHandler[T any] func(ctx context.Context, args T) (Result, error)

// Result is the tool execution result.
type Result = tools.Result

// ToolBuilder provides a fluent API for constructing tools.
type ToolBuilder struct {
	name                 string
	description          string
	params               map[string]*schema.ParamSchema
	paramOrder           []string
	handler              Handler
	confirmationMode     string
	requiresConfirmation bool
	examples             []Example
	tags                 []string
	deprecated           bool
	deprecationMessage   string
	version              string
	metadata             map[string]any
}

// Example represents a usage example for documentation.
type Example struct {
	Name        string
	Description string
	Args        map[string]any
	Expected    string
}

// NewTool creates a new tool builder with the given name.
func NewTool(name string) *ToolBuilder {
	return &ToolBuilder{
		name:     name,
		params:   make(map[string]*schema.ParamSchema),
		metadata: make(map[string]any),
	}
}

// Description sets the tool description.
func (b *ToolBuilder) Description(desc string) *ToolBuilder {
	b.description = desc
	return b
}

// Param adds a parameter to the tool using a schema builder.
func (b *ToolBuilder) Param(name string, schemaBuilder *schema.Builder) *ToolBuilder {
	b.params[name] = schemaBuilder.Build(name)
	b.paramOrder = append(b.paramOrder, name)
	return b
}

// RequiredParam adds a required parameter.
func (b *ToolBuilder) RequiredParam(name string, schemaBuilder *schema.Builder) *ToolBuilder {
	return b.Param(name, schemaBuilder.Required())
}

// OptionalParam adds an optional parameter.
func (b *ToolBuilder) OptionalParam(name string, schemaBuilder *schema.Builder) *ToolBuilder {
	return b.Param(name, schemaBuilder.Optional())
}

// Execute sets the handler function for the tool.
func (b *ToolBuilder) Execute(handler Handler) *ToolBuilder {
	b.handler = handler
	return b
}

// RequiresConfirmationOn specifies when confirmation is required.
func (b *ToolBuilder) RequiresConfirmationOn(mode string) *ToolBuilder {
	b.confirmationMode = mode
	b.requiresConfirmation = true
	return b
}

// RequiresConfirmationAlways marks the tool as always requiring confirmation.
func (b *ToolBuilder) RequiresConfirmationAlways() *ToolBuilder {
	b.requiresConfirmation = true
	b.confirmationMode = ""
	return b
}

// NoConfirmation marks the tool as never requiring confirmation.
func (b *ToolBuilder) NoConfirmation() *ToolBuilder {
	b.requiresConfirmation = false
	return b
}

// Example adds a usage example.
func (b *ToolBuilder) Example(name, description string, args map[string]any, expected string) *ToolBuilder {
	b.examples = append(b.examples, Example{
		Name:        name,
		Description: description,
		Args:        args,
		Expected:    expected,
	})
	return b
}

// Tags adds tags for categorization.
func (b *ToolBuilder) Tags(tags ...string) *ToolBuilder {
	b.tags = append(b.tags, tags...)
	return b
}

// Deprecated marks the tool as deprecated.
func (b *ToolBuilder) Deprecated(message string) *ToolBuilder {
	b.deprecated = true
	b.deprecationMessage = message
	return b
}

// Version sets the tool version.
func (b *ToolBuilder) Version(v string) *ToolBuilder {
	b.version = v
	return b
}

// Metadata adds custom metadata.
func (b *ToolBuilder) Metadata(key string, value any) *ToolBuilder {
	b.metadata[key] = value
	return b
}

// Build finalizes the tool and returns a Tool implementation.
func (b *ToolBuilder) Build() *FluentTool {
	return &FluentTool{
		builder: b,
	}
}

// BuildAndRegister builds the tool and registers it globally.
func (b *ToolBuilder) BuildAndRegister() *FluentTool {
	tool := b.Build()
	tools.Register(tool)
	return tool
}

// FluentTool is the concrete tool implementation.
type FluentTool struct {
	builder *ToolBuilder
}

// Name returns the tool name.
func (t *FluentTool) Name() string {
	return t.builder.name
}

// Description returns the tool description.
func (t *FluentTool) Description() string {
	return t.builder.description
}

// Schema returns the JSON schema for the tool parameters.
func (t *FluentTool) Schema() map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0)

	for name, param := range t.builder.params {
		properties[name] = param.ToJSONSchema()
		if param.Required {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// RequiresConfirmation checks if confirmation is needed for the given mode.
func (t *FluentTool) RequiresConfirmation(mode string) bool {
	if !t.builder.requiresConfirmation {
		return false
	}
	if t.builder.confirmationMode == "" {
		return true // Always require
	}
	return t.builder.confirmationMode == mode
}

// EstimatedCost provides default cost estimation for fluent tools.
func (t *FluentTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{
		TokensApprox: 100,
		LatencyMs:    50,
		RiskLevel:    "low",
	}
}

// IsConcurrencySafe returns false by default (conservative approach).
func (t *FluentTool) IsConcurrencySafe(args map[string]any) bool {
	return false
}

// IsReadOnly returns false by default (conservative approach).
func (t *FluentTool) IsReadOnly(args map[string]any) bool {
	return false
}

// IsDestructive returns false by default.
func (t *FluentTool) IsDestructive(args map[string]any) bool {
	return false
}

// Execute runs the tool with the given arguments.
func (t *FluentTool) Execute(ctx context.Context, rawArgs map[string]any) (Result, error) {
	if t.builder.handler == nil {
		return Result{IsError: true, Content: "tool has no handler"}, nil
	}

	args := schema.NewArgs(rawArgs, t.builder.params)
	if err := args.Validate(); err != nil {
		return Result{IsError: true, Content: fmt.Sprintf("validation error: %v", err)}, nil
	}

	return t.builder.handler(ctx, args)
}

// Examples returns the usage examples.
func (t *FluentTool) Examples() []Example {
	return t.builder.examples
}

// Tags returns the tool tags.
func (t *FluentTool) Tags() []string {
	return t.builder.tags
}

// IsDeprecated returns whether the tool is deprecated.
func (t *FluentTool) IsDeprecated() bool {
	return t.builder.deprecated
}

// DeprecationMessage returns the deprecation message.
func (t *FluentTool) DeprecationMessage() string {
	return t.builder.deprecationMessage
}

// Version returns the tool version.
func (t *FluentTool) Version() string {
	return t.builder.version
}

// Metadata returns the custom metadata.
func (t *FluentTool) Metadata() map[string]any {
	return t.builder.metadata
}

// ParamSchemas returns the parameter schemas.
func (t *FluentTool) ParamSchemas() map[string]*schema.ParamSchema {
	return t.builder.params
}

// ParamOrder returns the order in which parameters were defined.
func (t *FluentTool) ParamOrder() []string {
	return t.builder.paramOrder
}
