package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Args provides type-safe argument binding and extraction.
type Args struct {
	raw    map[string]any
	params map[string]*ParamSchema
	errors []error
}

// NewArgs creates a new Args instance from raw arguments and parameter schemas.
func NewArgs(raw map[string]any, params map[string]*ParamSchema) *Args {
	return &Args{
		raw:    raw,
		params: params,
	}
}

// Validate checks all arguments against their schemas.
func (a *Args) Validate() error {
	a.errors = nil

	for name, schema := range a.params {
		value, exists := a.raw[name]
		if !exists {
			if schema.Default != nil {
				a.raw[name] = schema.Default
				continue
			}
			value = nil
		}
		if err := schema.Validate(value); err != nil {
			a.errors = append(a.errors, err)
		}
	}

	if len(a.errors) > 0 {
		return fmt.Errorf("validation errors: %v", a.errors)
	}
	return nil
}

// String returns a string argument value.
func (a *Args) String(name string) string {
	v, _ := a.raw[name].(string)
	return v
}

// StringOr returns a string argument value or a default.
func (a *Args) StringOr(name, defaultValue string) string {
	if v, ok := a.raw[name].(string); ok {
		return v
	}
	return defaultValue
}

// Int returns an integer argument value.
func (a *Args) Int(name string) int {
	return int(a.Float(name))
}

// IntOr returns an integer argument value or a default.
func (a *Args) IntOr(name string, defaultValue int) int {
	if _, ok := a.raw[name]; ok {
		return a.Int(name)
	}
	return defaultValue
}

// Float returns a float argument value.
func (a *Args) Float(name string) float64 {
	switch v := a.raw[name].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

// FloatOr returns a float argument value or a default.
func (a *Args) FloatOr(name string, defaultValue float64) float64 {
	if _, ok := a.raw[name]; ok {
		return a.Float(name)
	}
	return defaultValue
}

// Bool returns a boolean argument value.
func (a *Args) Bool(name string) bool {
	switch v := a.raw[name].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}

// BoolOr returns a boolean argument value or a default.
func (a *Args) BoolOr(name string, defaultValue bool) bool {
	if _, ok := a.raw[name]; ok {
		return a.Bool(name)
	}
	return defaultValue
}

// Array returns an array argument value.
func (a *Args) Array(name string) []any {
	v, _ := a.raw[name].([]any)
	return v
}

// StringArray returns a string array argument value.
func (a *Args) StringArray(name string) []string {
	arr := a.Array(name)
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// Object returns an object argument value.
func (a *Args) Object(name string) map[string]any {
	v, _ := a.raw[name].(map[string]any)
	return v
}

// Has returns true if the argument exists and is not nil.
func (a *Args) Has(name string) bool {
	v, ok := a.raw[name]
	return ok && v != nil
}

// Raw returns the raw argument value.
func (a *Args) Raw(name string) any {
	return a.raw[name]
}

// RawMap returns the underlying raw arguments map.
func (a *Args) RawMap() map[string]any {
	return a.raw
}

// Bind unmarshals arguments into a struct using struct tags.
// Struct fields should be tagged with `arg:"name"` to specify the argument name.
func (a *Args) Bind(target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer to struct")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("target must be a pointer to struct")
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		argName := field.Tag.Get("arg")
		if argName == "" {
			continue
		}

		rawValue := a.raw[argName]
		if rawValue == nil {
			continue
		}

		fieldValue := v.Field(i)
		if !fieldValue.CanSet() {
			continue
		}

		if err := setFieldValue(fieldValue, rawValue); err != nil {
			return fmt.Errorf("failed to set field %s: %w", field.Name, err)
		}
	}

	return nil
}

func setFieldValue(field reflect.Value, value any) error {
	switch field.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			field.SetString(s)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := value.(type) {
		case float64:
			field.SetInt(int64(v))
		case int:
			field.SetInt(int64(v))
		case int64:
			field.SetInt(v)
		case json.Number:
			n, _ := v.Int64()
			field.SetInt(n)
		}
	case reflect.Float32, reflect.Float64:
		switch v := value.(type) {
		case float64:
			field.SetFloat(v)
		case float32:
			field.SetFloat(float64(v))
		case int:
			field.SetFloat(float64(v))
		case json.Number:
			f, _ := v.Float64()
			field.SetFloat(f)
		}
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			field.SetBool(b)
		}
	case reflect.Slice:
		return setSliceValue(field, value)
	case reflect.Map:
		if m, ok := value.(map[string]any); ok {
			field.Set(reflect.ValueOf(m))
		}
	default:
		return fmt.Errorf("unsupported field type: %s", field.Kind())
	}
	return nil
}

func setSliceValue(field reflect.Value, value any) error {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}

	elemType := field.Type().Elem()
	slice := reflect.MakeSlice(field.Type(), 0, len(arr))

	for _, item := range arr {
		elem := reflect.New(elemType).Elem()
		if err := setFieldValue(elem, item); err != nil {
			return err
		}
		slice = reflect.Append(slice, elem)
	}

	field.Set(slice)
	return nil
}

// MustBind is like Bind but panics on error.
func (a *Args) MustBind(target any) {
	if err := a.Bind(target); err != nil {
		panic(err)
	}
}
