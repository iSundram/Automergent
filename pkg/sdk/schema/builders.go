package schema

import "regexp"

// Builder provides a fluent API for constructing parameter schemas.
type Builder struct {
	schema *ParamSchema
}

// String creates a new string parameter builder.
func String() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeString}}
}

// Number creates a new number parameter builder.
func Number() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeNumber}}
}

// Integer creates a new integer parameter builder.
func Integer() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeInteger}}
}

// Boolean creates a new boolean parameter builder.
func Boolean() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeBoolean}}
}

// Array creates a new array parameter builder.
func Array() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeArray}}
}

// Object creates a new object parameter builder.
func Object() *Builder {
	return &Builder{schema: &ParamSchema{Type: TypeObject, Properties: make(map[string]*ParamSchema)}}
}

// Description sets the parameter description.
func (b *Builder) Description(desc string) *Builder {
	b.schema.Description = desc
	return b
}

// Required marks the parameter as required.
func (b *Builder) Required() *Builder {
	b.schema.Required = true
	return b
}

// Optional marks the parameter as optional (default).
func (b *Builder) Optional() *Builder {
	b.schema.Required = false
	return b
}

// Default sets the default value for the parameter.
func (b *Builder) Default(value any) *Builder {
	b.schema.Default = value
	return b
}

// Enum restricts the parameter to a set of allowed string values.
func (b *Builder) Enum(values ...string) *Builder {
	b.schema.Enum = values
	return b
}

// MinLength sets the minimum string length.
func (b *Builder) MinLength(n int) *Builder {
	b.schema.MinLength = &n
	return b
}

// MaxLength sets the maximum string length.
func (b *Builder) MaxLength(n int) *Builder {
	b.schema.MaxLength = &n
	return b
}

// Pattern sets a regex pattern for string validation.
func (b *Builder) Pattern(pattern string) *Builder {
	b.schema.Pattern = regexp.MustCompile(pattern)
	return b
}

// Min sets the minimum numeric value (inclusive).
func (b *Builder) Min(n float64) *Builder {
	b.schema.Minimum = &n
	return b
}

// Max sets the maximum numeric value (inclusive).
func (b *Builder) Max(n float64) *Builder {
	b.schema.Maximum = &n
	return b
}

// ExclusiveMin sets the exclusive minimum numeric value.
func (b *Builder) ExclusiveMin(n float64) *Builder {
	b.schema.ExclusiveMinimum = &n
	return b
}

// ExclusiveMax sets the exclusive maximum numeric value.
func (b *Builder) ExclusiveMax(n float64) *Builder {
	b.schema.ExclusiveMaximum = &n
	return b
}

// MultipleOf requires the value to be a multiple of n.
func (b *Builder) MultipleOf(n float64) *Builder {
	b.schema.MultipleOf = &n
	return b
}

// Range sets both minimum and maximum values (inclusive).
func (b *Builder) Range(min, max float64) *Builder {
	b.schema.Minimum = &min
	b.schema.Maximum = &max
	return b
}

// Items sets the schema for array items.
func (b *Builder) Items(itemSchema *Builder) *Builder {
	b.schema.Items = itemSchema.Build("")
	return b
}

// MinItems sets the minimum number of array items.
func (b *Builder) MinItems(n int) *Builder {
	b.schema.MinItems = &n
	return b
}

// MaxItems sets the maximum number of array items.
func (b *Builder) MaxItems(n int) *Builder {
	b.schema.MaxItems = &n
	return b
}

// Property adds a property to an object schema.
func (b *Builder) Property(name string, propSchema *Builder) *Builder {
	if b.schema.Properties == nil {
		b.schema.Properties = make(map[string]*ParamSchema)
	}
	b.schema.Properties[name] = propSchema.Build(name)
	return b
}

// Build finalizes the schema with the given parameter name.
func (b *Builder) Build(name string) *ParamSchema {
	b.schema.Name = name
	return b.schema
}

// Clone creates a deep copy of the builder for modification.
func (b *Builder) Clone() *Builder {
	newSchema := *b.schema
	return &Builder{schema: &newSchema}
}
