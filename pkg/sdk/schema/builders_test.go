package schema

import (
	"testing"
)

func TestStringBuilder(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
		value   any
		wantErr bool
	}{
		{
			name:    "valid string",
			builder: String().Required(),
			value:   "hello",
			wantErr: false,
		},
		{
			name:    "nil required",
			builder: String().Required(),
			value:   nil,
			wantErr: true,
		},
		{
			name:    "min length valid",
			builder: String().MinLength(3),
			value:   "hello",
			wantErr: false,
		},
		{
			name:    "min length invalid",
			builder: String().MinLength(10),
			value:   "hi",
			wantErr: true,
		},
		{
			name:    "max length valid",
			builder: String().MaxLength(10),
			value:   "hello",
			wantErr: false,
		},
		{
			name:    "max length invalid",
			builder: String().MaxLength(3),
			value:   "hello",
			wantErr: true,
		},
		{
			name:    "pattern valid",
			builder: String().Pattern(`^\d+$`),
			value:   "123",
			wantErr: false,
		},
		{
			name:    "pattern invalid",
			builder: String().Pattern(`^\d+$`),
			value:   "abc",
			wantErr: true,
		},
		{
			name:    "enum valid",
			builder: String().Enum("a", "b", "c"),
			value:   "b",
			wantErr: false,
		},
		{
			name:    "enum invalid",
			builder: String().Enum("a", "b", "c"),
			value:   "d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.builder.Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNumberBuilder(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
		value   any
		wantErr bool
	}{
		{
			name:    "valid number",
			builder: Number(),
			value:   42.5,
			wantErr: false,
		},
		{
			name:    "min valid",
			builder: Number().Min(0),
			value:   10.0,
			wantErr: false,
		},
		{
			name:    "min invalid",
			builder: Number().Min(0),
			value:   -5.0,
			wantErr: true,
		},
		{
			name:    "max valid",
			builder: Number().Max(100),
			value:   50.0,
			wantErr: false,
		},
		{
			name:    "max invalid",
			builder: Number().Max(100),
			value:   150.0,
			wantErr: true,
		},
		{
			name:    "range valid",
			builder: Number().Range(0, 100),
			value:   50.0,
			wantErr: false,
		},
		{
			name:    "range invalid low",
			builder: Number().Range(0, 100),
			value:   -10.0,
			wantErr: true,
		},
		{
			name:    "range invalid high",
			builder: Number().Range(0, 100),
			value:   150.0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.builder.Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIntegerBuilder(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
		value   any
		wantErr bool
	}{
		{
			name:    "valid integer",
			builder: Integer(),
			value:   42.0,
			wantErr: false,
		},
		{
			name:    "float is invalid",
			builder: Integer(),
			value:   42.5,
			wantErr: true,
		},
		{
			name:    "int type",
			builder: Integer(),
			value:   42,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.builder.Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBooleanBuilder(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{"true", true, false},
		{"false", false, false},
		{"string", "true", true},
		{"number", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := Boolean().Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestArrayBuilder(t *testing.T) {
	tests := []struct {
		name    string
		builder *Builder
		value   any
		wantErr bool
	}{
		{
			name:    "valid array",
			builder: Array(),
			value:   []any{"a", "b"},
			wantErr: false,
		},
		{
			name:    "min items valid",
			builder: Array().MinItems(2),
			value:   []any{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "min items invalid",
			builder: Array().MinItems(3),
			value:   []any{"a"},
			wantErr: true,
		},
		{
			name:    "max items valid",
			builder: Array().MaxItems(5),
			value:   []any{"a", "b"},
			wantErr: false,
		},
		{
			name:    "max items invalid",
			builder: Array().MaxItems(2),
			value:   []any{"a", "b", "c", "d"},
			wantErr: true,
		},
		{
			name:    "items schema valid",
			builder: Array().Items(String()),
			value:   []any{"a", "b"},
			wantErr: false,
		},
		{
			name:    "items schema invalid",
			builder: Array().Items(String()),
			value:   []any{"a", 123},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.builder.Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestObjectBuilder(t *testing.T) {
	builder := Object().
		Property("name", String().Required()).
		Property("age", Integer())

	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{
			name: "valid object",
			value: map[string]any{
				"name": "John",
				"age":  30.0,
			},
			wantErr: false,
		},
		{
			name: "missing required",
			value: map[string]any{
				"age": 30.0,
			},
			wantErr: true,
		},
		{
			name:    "not an object",
			value:   "string",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := builder.Build("test")
			err := schema.Validate(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultValue(t *testing.T) {
	schema := String().Default("default_value").Build("test")

	if schema.Default != "default_value" {
		t.Errorf("expected default 'default_value', got %v", schema.Default)
	}
}

func TestDescription(t *testing.T) {
	schema := String().Description("A test parameter").Build("test")

	if schema.Description != "A test parameter" {
		t.Errorf("expected description 'A test parameter', got %s", schema.Description)
	}
}

func TestToJSONSchema(t *testing.T) {
	schema := String().
		Required().
		MinLength(1).
		MaxLength(100).
		Pattern(`^[a-z]+$`).
		Description("A lowercase string").
		Build("test")

	jsonSchema := schema.ToJSONSchema()

	if jsonSchema["type"] != "string" {
		t.Errorf("expected type 'string', got %v", jsonSchema["type"])
	}
	if jsonSchema["description"] != "A lowercase string" {
		t.Errorf("expected description, got %v", jsonSchema["description"])
	}
	if jsonSchema["minLength"] != 1 {
		t.Errorf("expected minLength 1, got %v", jsonSchema["minLength"])
	}
	if jsonSchema["maxLength"] != 100 {
		t.Errorf("expected maxLength 100, got %v", jsonSchema["maxLength"])
	}
	if jsonSchema["pattern"] != `^[a-z]+$` {
		t.Errorf("expected pattern, got %v", jsonSchema["pattern"])
	}
}

func TestClone(t *testing.T) {
	original := String().Description("original")
	cloned := original.Clone().Description("cloned")

	originalSchema := original.Build("original")
	clonedSchema := cloned.Build("cloned")

	if originalSchema.Description != "original" {
		t.Errorf("original description changed")
	}
	if clonedSchema.Description != "cloned" {
		t.Errorf("cloned description not updated")
	}
}
