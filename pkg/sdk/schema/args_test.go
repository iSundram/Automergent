package schema

import (
	"testing"
)

func TestArgs_String(t *testing.T) {
	args := NewArgs(map[string]any{
		"name": "John",
		"age":  30.0,
	}, nil)

	if got := args.String("name"); got != "John" {
		t.Errorf("String() = %v, want John", got)
	}
	if got := args.String("missing"); got != "" {
		t.Errorf("String() for missing = %v, want empty", got)
	}
}

func TestArgs_StringOr(t *testing.T) {
	args := NewArgs(map[string]any{
		"name": "John",
	}, nil)

	if got := args.StringOr("name", "default"); got != "John" {
		t.Errorf("StringOr() = %v, want John", got)
	}
	if got := args.StringOr("missing", "default"); got != "default" {
		t.Errorf("StringOr() = %v, want default", got)
	}
}

func TestArgs_Int(t *testing.T) {
	args := NewArgs(map[string]any{
		"count": 42.0,
		"int":   int(10),
	}, nil)

	if got := args.Int("count"); got != 42 {
		t.Errorf("Int() = %v, want 42", got)
	}
	if got := args.Int("int"); got != 10 {
		t.Errorf("Int() = %v, want 10", got)
	}
}

func TestArgs_Float(t *testing.T) {
	args := NewArgs(map[string]any{
		"value":   3.14,
		"integer": int(42),
	}, nil)

	if got := args.Float("value"); got != 3.14 {
		t.Errorf("Float() = %v, want 3.14", got)
	}
	if got := args.Float("integer"); got != 42.0 {
		t.Errorf("Float() = %v, want 42.0", got)
	}
}

func TestArgs_Bool(t *testing.T) {
	args := NewArgs(map[string]any{
		"enabled":  true,
		"disabled": false,
		"strTrue":  "true",
		"strFalse": "false",
		"numOne":   1,
		"numZero":  0,
	}, nil)

	tests := []struct {
		key  string
		want bool
	}{
		{"enabled", true},
		{"disabled", false},
		{"strTrue", true},
		{"strFalse", false},
		{"numOne", true},
		{"numZero", false},
	}

	for _, tt := range tests {
		if got := args.Bool(tt.key); got != tt.want {
			t.Errorf("Bool(%s) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestArgs_Array(t *testing.T) {
	args := NewArgs(map[string]any{
		"items": []any{"a", "b", "c"},
	}, nil)

	got := args.Array("items")
	if len(got) != 3 {
		t.Errorf("Array() len = %v, want 3", len(got))
	}
}

func TestArgs_StringArray(t *testing.T) {
	args := NewArgs(map[string]any{
		"tags": []any{"go", "testing", 123}, // mixed types
	}, nil)

	got := args.StringArray("tags")
	if len(got) != 2 { // only strings should be included
		t.Errorf("StringArray() len = %v, want 2", len(got))
	}
	if got[0] != "go" || got[1] != "testing" {
		t.Errorf("StringArray() = %v, want [go, testing]", got)
	}
}

func TestArgs_Object(t *testing.T) {
	args := NewArgs(map[string]any{
		"config": map[string]any{
			"key": "value",
		},
	}, nil)

	got := args.Object("config")
	if got["key"] != "value" {
		t.Errorf("Object() = %v, want map with key=value", got)
	}
}

func TestArgs_Has(t *testing.T) {
	args := NewArgs(map[string]any{
		"present": "value",
		"nil":     nil,
	}, nil)

	if !args.Has("present") {
		t.Error("Has(present) = false, want true")
	}
	if args.Has("nil") {
		t.Error("Has(nil) = true, want false")
	}
	if args.Has("missing") {
		t.Error("Has(missing) = true, want false")
	}
}

func TestArgs_Validate(t *testing.T) {
	params := map[string]*ParamSchema{
		"name": String().Required().Build("name"),
		"age":  Integer().Build("age"),
	}

	tests := []struct {
		name    string
		raw     map[string]any
		wantErr bool
	}{
		{
			name:    "valid",
			raw:     map[string]any{"name": "John", "age": 30.0},
			wantErr: false,
		},
		{
			name:    "missing required",
			raw:     map[string]any{"age": 30.0},
			wantErr: true,
		},
		{
			name:    "with default",
			raw:     map[string]any{"name": "John"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := NewArgs(tt.raw, params)
			err := args.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestArgs_Bind(t *testing.T) {
	type TestArgs struct {
		Name    string   `arg:"name"`
		Age     int      `arg:"age"`
		Active  bool     `arg:"active"`
		Score   float64  `arg:"score"`
		Tags    []string `arg:"tags"`
		Ignored string   // no tag
	}

	args := NewArgs(map[string]any{
		"name":   "John",
		"age":    30.0,
		"active": true,
		"score":  95.5,
		"tags":   []any{"a", "b"},
	}, nil)

	var target TestArgs
	if err := args.Bind(&target); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}

	if target.Name != "John" {
		t.Errorf("Name = %v, want John", target.Name)
	}
	if target.Age != 30 {
		t.Errorf("Age = %v, want 30", target.Age)
	}
	if !target.Active {
		t.Error("Active = false, want true")
	}
	if target.Score != 95.5 {
		t.Errorf("Score = %v, want 95.5", target.Score)
	}
	if len(target.Tags) != 2 {
		t.Errorf("Tags len = %v, want 2", len(target.Tags))
	}
}

func TestArgs_BindErrors(t *testing.T) {
	args := NewArgs(map[string]any{}, nil)

	// Non-pointer
	var s struct{}
	if err := args.Bind(s); err == nil {
		t.Error("expected error for non-pointer")
	}

	// Nil pointer
	if err := args.Bind(nil); err == nil {
		t.Error("expected error for nil")
	}

	// Non-struct pointer
	var i int
	if err := args.Bind(&i); err == nil {
		t.Error("expected error for non-struct")
	}
}

func TestArgs_ValidateWithDefaults(t *testing.T) {
	params := map[string]*ParamSchema{
		"encoding": String().Default("utf-8").Build("encoding"),
	}

	args := NewArgs(map[string]any{}, params)
	if err := args.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}

	// After validation, default should be applied
	if args.String("encoding") != "utf-8" {
		t.Errorf("encoding = %v, want utf-8", args.String("encoding"))
	}
}
