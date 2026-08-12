package builder

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/pkg/sdk/schema"
)

// TypedToolBuilder provides type-safe tool construction with struct binding.
type TypedToolBuilder[T any] struct {
	base    *ToolBuilder
	handler GenericHandler[T]
}

// NewTypedTool creates a new typed tool builder.
func NewTypedTool[T any](name string) *TypedToolBuilder[T] {
	return &TypedToolBuilder[T]{
		base: NewTool(name),
	}
}

// Description sets the tool description.
func (b *TypedToolBuilder[T]) Description(desc string) *TypedToolBuilder[T] {
	b.base.Description(desc)
	return b
}

// Param adds a parameter to the tool.
func (b *TypedToolBuilder[T]) Param(name string, schemaBuilder *schema.Builder) *TypedToolBuilder[T] {
	b.base.Param(name, schemaBuilder)
	return b
}

// RequiredParam adds a required parameter.
func (b *TypedToolBuilder[T]) RequiredParam(name string, schemaBuilder *schema.Builder) *TypedToolBuilder[T] {
	b.base.RequiredParam(name, schemaBuilder)
	return b
}

// OptionalParam adds an optional parameter.
func (b *TypedToolBuilder[T]) OptionalParam(name string, schemaBuilder *schema.Builder) *TypedToolBuilder[T] {
	b.base.OptionalParam(name, schemaBuilder)
	return b
}

// Execute sets the typed handler function.
func (b *TypedToolBuilder[T]) Execute(handler GenericHandler[T]) *TypedToolBuilder[T] {
	b.handler = handler
	return b
}

// RequiresConfirmationOn specifies when confirmation is required.
func (b *TypedToolBuilder[T]) RequiresConfirmationOn(mode string) *TypedToolBuilder[T] {
	b.base.RequiresConfirmationOn(mode)
	return b
}

// RequiresConfirmationAlways marks the tool as always requiring confirmation.
func (b *TypedToolBuilder[T]) RequiresConfirmationAlways() *TypedToolBuilder[T] {
	b.base.RequiresConfirmationAlways()
	return b
}

// NoConfirmation marks the tool as never requiring confirmation.
func (b *TypedToolBuilder[T]) NoConfirmation() *TypedToolBuilder[T] {
	b.base.NoConfirmation()
	return b
}

// Example adds a usage example.
func (b *TypedToolBuilder[T]) Example(name, description string, args map[string]any, expected string) *TypedToolBuilder[T] {
	b.base.Example(name, description, args, expected)
	return b
}

// Tags adds tags for categorization.
func (b *TypedToolBuilder[T]) Tags(tags ...string) *TypedToolBuilder[T] {
	b.base.Tags(tags...)
	return b
}

// Deprecated marks the tool as deprecated.
func (b *TypedToolBuilder[T]) Deprecated(message string) *TypedToolBuilder[T] {
	b.base.Deprecated(message)
	return b
}

// Version sets the tool version.
func (b *TypedToolBuilder[T]) Version(v string) *TypedToolBuilder[T] {
	b.base.Version(v)
	return b
}

// Metadata adds custom metadata.
func (b *TypedToolBuilder[T]) Metadata(key string, value any) *TypedToolBuilder[T] {
	b.base.Metadata(key, value)
	return b
}

// Build finalizes the tool.
func (b *TypedToolBuilder[T]) Build() *FluentTool {
	// Wrap the typed handler
	b.base.handler = func(ctx context.Context, args *schema.Args) (Result, error) {
		var typedArgs T
		if err := args.Bind(&typedArgs); err != nil {
			return Result{IsError: true, Content: fmt.Sprintf("argument binding error: %v", err)}, nil
		}
		return b.handler(ctx, typedArgs)
	}
	return b.base.Build()
}

// BuildAndRegister builds and registers the tool globally.
func (b *TypedToolBuilder[T]) BuildAndRegister() *FluentTool {
	return b.base.BuildAndRegister()
}
