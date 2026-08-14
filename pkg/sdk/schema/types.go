// Package schema provides type-safe schema definitions for tool parameters.
package schema

import (
	"fmt"
	"regexp"
)

const (
	defaultMaxStringLength = 64 * 1024
	defaultMaxArrayItems   = 1000
)

// Type represents a JSON schema type.
type Type string

const (
	TypeString  Type = "string"
	TypeNumber  Type = "number"
	TypeInteger Type = "integer"
	TypeBoolean Type = "boolean"
	TypeArray   Type = "array"
	TypeObject  Type = "object"
)

// ParamSchema defines a parameter schema with validation rules.
type ParamSchema struct {
	Name        string
	Type        Type
	Description string
	Required    bool
	Default     any

	// String constraints
	MinLength *int
	MaxLength *int
	Pattern   *regexp.Regexp
	Enum      []string

	// Numeric constraints
	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum *float64
	ExclusiveMaximum *float64
	MultipleOf       *float64

	// Array constraints
	Items    *ParamSchema
	MinItems *int
	MaxItems *int

	// Object constraints
	Properties map[string]*ParamSchema
}

// ToJSONSchema converts the ParamSchema to JSON Schema format.
func (p *ParamSchema) ToJSONSchema() map[string]any {
	schema := map[string]any{
		"type": string(p.Type),
	}

	if p.Description != "" {
		schema["description"] = p.Description
	}
	if p.Default != nil {
		schema["default"] = p.Default
	}
	if len(p.Enum) > 0 {
		schema["enum"] = p.Enum
	}

	// String constraints
	if p.MinLength != nil {
		schema["minLength"] = *p.MinLength
	}
	if p.MaxLength != nil {
		schema["maxLength"] = *p.MaxLength
	}
	if p.Pattern != nil {
		schema["pattern"] = p.Pattern.String()
	}

	// Numeric constraints
	if p.Minimum != nil {
		schema["minimum"] = *p.Minimum
	}
	if p.Maximum != nil {
		schema["maximum"] = *p.Maximum
	}
	if p.ExclusiveMinimum != nil {
		schema["exclusiveMinimum"] = *p.ExclusiveMinimum
	}
	if p.ExclusiveMaximum != nil {
		schema["exclusiveMaximum"] = *p.ExclusiveMaximum
	}
	if p.MultipleOf != nil {
		schema["multipleOf"] = *p.MultipleOf
	}

	// Array constraints
	if p.Items != nil {
		schema["items"] = p.Items.ToJSONSchema()
	}
	if p.MinItems != nil {
		schema["minItems"] = *p.MinItems
	}
	if p.MaxItems != nil {
		schema["maxItems"] = *p.MaxItems
	}

	// Object constraints
	if len(p.Properties) > 0 {
		props := make(map[string]any)
		for name, prop := range p.Properties {
			props[name] = prop.ToJSONSchema()
		}
		schema["properties"] = props
	}

	return schema
}

// Validate checks if a value conforms to this schema.
func (p *ParamSchema) Validate(value any) error {
	if value == nil {
		if p.Required {
			return fmt.Errorf("parameter %q is required", p.Name)
		}
		return nil
	}

	switch p.Type {
	case TypeString:
		return p.validateString(value)
	case TypeNumber, TypeInteger:
		return p.validateNumber(value)
	case TypeBoolean:
		return p.validateBoolean(value)
	case TypeArray:
		return p.validateArray(value)
	case TypeObject:
		return p.validateObject(value)
	}

	return nil
}

func (p *ParamSchema) validateString(value any) error {
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("parameter %q must be a string, got %T", p.Name, value)
	}

	if p.MinLength != nil && len(s) < *p.MinLength {
		return fmt.Errorf("parameter %q must be at least %d characters", p.Name, *p.MinLength)
	}
	maxLength := defaultMaxStringLength
	if p.MaxLength != nil {
		maxLength = *p.MaxLength
	}
	if len(s) > maxLength {
		return fmt.Errorf("parameter %q must be at most %d characters", p.Name, maxLength)
	}
	if p.Pattern != nil && !p.Pattern.MatchString(s) {
		return fmt.Errorf("parameter %q must match pattern %s", p.Name, p.Pattern.String())
	}
	if len(p.Enum) > 0 {
		found := false
		for _, e := range p.Enum {
			if s == e {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parameter %q must be one of %v", p.Name, p.Enum)
		}
	}

	return nil
}

func (p *ParamSchema) validateNumber(value any) error {
	var n float64
	switch v := value.(type) {
	case float64:
		n = v
	case float32:
		n = float64(v)
	case int:
		n = float64(v)
	case int32:
		n = float64(v)
	case int64:
		n = float64(v)
	default:
		return fmt.Errorf("parameter %q must be a number, got %T", p.Name, value)
	}

	if p.Type == TypeInteger && n != float64(int64(n)) {
		return fmt.Errorf("parameter %q must be an integer", p.Name)
	}

	if p.Minimum != nil && n < *p.Minimum {
		return fmt.Errorf("parameter %q must be >= %v", p.Name, *p.Minimum)
	}
	if p.Maximum != nil && n > *p.Maximum {
		return fmt.Errorf("parameter %q must be <= %v", p.Name, *p.Maximum)
	}
	if p.ExclusiveMinimum != nil && n <= *p.ExclusiveMinimum {
		return fmt.Errorf("parameter %q must be > %v", p.Name, *p.ExclusiveMinimum)
	}
	if p.ExclusiveMaximum != nil && n >= *p.ExclusiveMaximum {
		return fmt.Errorf("parameter %q must be < %v", p.Name, *p.ExclusiveMaximum)
	}
	if p.MultipleOf != nil && float64(int64(n/(*p.MultipleOf)))*(*p.MultipleOf) != n {
		return fmt.Errorf("parameter %q must be a multiple of %v", p.Name, *p.MultipleOf)
	}

	return nil
}

func (p *ParamSchema) validateBoolean(value any) error {
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("parameter %q must be a boolean, got %T", p.Name, value)
	}
	return nil
}

func (p *ParamSchema) validateArray(value any) error {
	arr, ok := value.([]any)
	if !ok {
		return fmt.Errorf("parameter %q must be an array, got %T", p.Name, value)
	}

	if p.MinItems != nil && len(arr) < *p.MinItems {
		return fmt.Errorf("parameter %q must have at least %d items", p.Name, *p.MinItems)
	}
	maxItems := defaultMaxArrayItems
	if p.MaxItems != nil {
		maxItems = *p.MaxItems
	}
	if len(arr) > maxItems {
		return fmt.Errorf("parameter %q must have at most %d items", p.Name, maxItems)
	}

	if p.Items != nil {
		for i, item := range arr {
			if err := p.Items.Validate(item); err != nil {
				return fmt.Errorf("parameter %q[%d]: %w", p.Name, i, err)
			}
		}
	}

	return nil
}

func (p *ParamSchema) validateObject(value any) error {
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("parameter %q must be an object, got %T", p.Name, value)
	}

	if len(p.Properties) > 0 {
		for key := range obj {
			if _, exists := p.Properties[key]; !exists {
				return fmt.Errorf("parameter %q has unknown property %q", p.Name, key)
			}
		}
	}

	for name, prop := range p.Properties {
		if err := prop.Validate(obj[name]); err != nil {
			return fmt.Errorf("parameter %q.%s: %w", p.Name, name, err)
		}
	}

	return nil
}
