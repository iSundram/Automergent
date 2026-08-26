package config

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Schema defines validation rules for configuration.
type Schema struct {
	Fields map[string]FieldSchema
}

// FieldSchema defines validation rules for a single field.
type FieldSchema struct {
	Type        FieldType
	Required    bool
	Default     any
	Min         *float64
	Max         *float64
	MinLen      *int
	MaxLen      *int
	Pattern     *regexp.Regexp
	Enum        []string
	Items       *FieldSchema // For arrays
	Deprecated  bool
	Description string
	Sensitive   bool // Marks secrets
}

// FieldType represents the expected type of a field.
type FieldType int

const (
	TypeString FieldType = iota
	TypeInt
	TypeFloat
	TypeBool
	TypeArray
	TypeMap
	TypeDuration
	TypeSize
)

// String returns the type name.
func (t FieldType) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeArray:
		return "array"
	case TypeMap:
		return "map"
	case TypeDuration:
		return "duration"
	case TypeSize:
		return "size"
	default:
		return "unknown"
	}
}

// ValidationError represents a single validation error.
type ValidationError struct {
	Field   string
	Message string
	Value   any
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (value: %v)", e.Field, e.Message, e.Value)
}

// DefaultSchema returns the default configuration schema.
func DefaultSchema() *Schema {
	minZero := 0.0
	minOne := 1.0
	maxOne := 1.0
	minLenOne := 1

	return &Schema{
		Fields: map[string]FieldSchema{
			"provider": {
				Type:        TypeString,
				Required:    true,
				Default:     "google",
				Enum:        ProviderNames(),
				Description: "AI provider to use",
			},
			"model": {
				Type:        TypeString,
				Required:    true,
				Default:     "gemini-3.6-flash",
				MinLen:      &minLenOne,
				Description: "Model name",
			},
			"mode": {
				Type:        TypeString,
				Required:    true,
				Default:     "edit",
				Enum:        []string{"edit", "code", "chat", "agent"},
				Description: "Operating mode",
			},
			"theme": {
				Type:        TypeString,
				Default:     "modern",
				Enum:        []string{"catppuccin", "dracula", "nord", "gruvbox", "onedark", "monokai", "modern", "default"},
				Description: "UI theme",
			},
			"keybindings": {
				Type:        TypeString,
				Default:     "default",
				Enum:        []string{"default", "vim", "emacs"},
				Description: "Keybinding preset",
			},
			"layout": {
				Type:        TypeString,
				Default:     "default",
				Enum:        []string{"default", "minimal", "full", "split"},
				Description: "UI layout",
			},
			"autoSave": {
				Type:        TypeBool,
				Default:     true,
				Description: "Enable auto-save",
			},
			"checkpointInterval": {
				Type:        TypeInt,
				Default:     5,
				Min:         &minOne,
				Description: "Checkpoint interval in minutes",
			},
			"sessionDir": {
				Type:        TypeString,
				Description: "Session storage directory",
			},
			"maxSessions": {
				Type:        TypeInt,
				Default:     100,
				Min:         &minOne,
				Description: "Maximum number of sessions to keep",
			},
			"maxSessionAge": {
				Type:        TypeDuration,
				Default:     "",
				Description: "Maximum age of sessions (e.g., 30d)",
			},
			"maxContextTokens": {
				Type:        TypeInt,
				Default:     128000,
				Min:         &minOne,
				Description: "Maximum context window tokens",
			},
			"warnAtContextFraction": {
				Type:        TypeFloat,
				Default:     0.8,
				Min:         &minZero,
				Max:         &maxOne,
				Description: "Context usage warning threshold (0-1)",
			},
			"autoCompressAt": {
				Type:        TypeFloat,
				Default:     0.9,
				Min:         &minZero,
				Max:         &maxOne,
				Description: "Auto-compress threshold (0-1)",
			},
			"compressionKeepRecent": {
				Type:        TypeInt,
				Default:     10,
				Min:         &minZero,
				Description: "Recent messages to keep when compressing",
			},
			"maxAutoReadFileSize": {
				Type:        TypeInt,
				Default:     524288, // 512KB
				Min:         &minZero,
				Description: "Maximum file size for auto-read (bytes)",
			},
			"maxTreeFiles": {
				Type:        TypeInt,
				Default:     1000,
				Min:         &minOne,
				Description: "Maximum files in tree view",
			},
			"maxTreeDepth": {
				Type:        TypeInt,
				Default:     10,
				Min:         &minOne,
				Description: "Maximum tree depth",
			},
			"excludePatterns": {
				Type:        TypeArray,
				Items:       &FieldSchema{Type: TypeString},
				Description: "Glob patterns to exclude",
			},
			"noAnimation": {
				Type:        TypeBool,
				Default:     false,
				Description: "Disable animations",
			},
			"noColor": {
				Type:        TypeBool,
				Default:     false,
				Description: "Disable colors",
			},
			"noTui": {
				Type:        TypeBool,
				Default:     false,
				Description: "Disable TUI mode",
			},
			"quiet": {
				Type:        TypeBool,
				Default:     false,
				Description: "Quiet mode",
			},
			"verbose": {
				Type:        TypeBool,
				Default:     false,
				Description: "Verbose output",
			},
			"reasoningPreAnalysis": {
				Type:        TypeBool,
				Default:     false,
				Description: "Enable reasoning pre-analysis for each prompt",
			},

			"security.sandbox": {
				Type:        TypeString,
				Default:     "auto",
				Enum:        []string{"off", "auto", "strict"},
				Description: "Sandbox mode",
			},
			"security.requireGitForAutoModes": {
				Type:        TypeBool,
				Default:     true,
				Description: "Require Git for auto modes",
			},
			"log.level": {
				Type:        TypeString,
				Default:     "warn",
				Enum:        []string{"debug", "info", "warn", "error"},
				Description: "Log level",
			},
			"log.file": {
				Type:        TypeString,
				Description: "Log file path",
			},
			"log.maxSize": {
				Type:        TypeSize,
				Default:     "50MB",
				Description: "Maximum log file size",
			},
			"log.maxBackups": {
				Type:        TypeInt,
				Default:     3,
				Min:         &minZero,
				Description: "Maximum log backups",
			},
			"lsp.enabled": {
				Type:        TypeBool,
				Default:     true,
				Description: "Enable LSP integration",
			},
			"lsp.startupTimeout": {
				Type:        TypeDuration,
				Default:     "10s",
				Description: "LSP server startup timeout",
			},
			"lsp.requestTimeout": {
				Type:        TypeDuration,
				Default:     "5s",
				Description: "LSP request timeout",
			},
			"telemetry": {
				Type:        TypeBool,
				Default:     true,
				Description: "Enable telemetry",
			},
			"zeroDataRetention": {
				Type:        TypeBool,
				Default:     false,
				Description: "Zero data retention mode",
			},
			"noUpdateCheck": {
				Type:        TypeBool,
				Default:     false,
				Description: "Disable update checks",
			},
			"diagnostics.enabled": {
				Type:        TypeBool,
				Default:     true,
				Description: "Enable diagnostics",
			},
			"diagnostics.showInRead": {
				Type:        TypeBool,
				Default:     true,
				Description: "Show diagnostics in file reads",
			},
			"diagnostics.blockOnError": {
				Type:        TypeBool,
				Default:     true,
				Description: "Block on diagnostic errors",
			},
			"notifications.desktop": {
				Type:        TypeBool,
				Default:     false,
				Description: "Enable desktop notifications",
			},
			"notifications.bell": {
				Type:        TypeBool,
				Default:     true,
				Description: "Enable terminal bell",
			},
			"notifications.contextWarning": {
				Type:        TypeBool,
				Default:     true,
				Description: "Warn on context limits",
			},
		},
	}
}

// Validate validates a config against the schema.
func (s *Schema) Validate(cfg *Config) []ValidationError {
	var errs []ValidationError

	// Convert config to map for easier validation
	data, err := configToMap(cfg)
	if err != nil {
		return []ValidationError{{Message: "failed to convert config"}}
	}

	// Validate each field
	for path, fieldSchema := range s.Fields {
		value := getNestedValue(data, path)

		// Check required
		if fieldSchema.Required && value == nil {
			errs = append(errs, ValidationError{
				Field:   path,
				Message: "required field is missing",
			})
			continue
		}

		if value == nil {
			continue
		}

		// Type validation
		if err := s.validateType(path, value, fieldSchema); err != nil {
			errs = append(errs, *err)
			continue
		}

		// Range validation
		if err := s.validateRange(path, value, fieldSchema); err != nil {
			errs = append(errs, *err)
		}

		// Enum validation
		if err := s.validateEnum(path, value, fieldSchema); err != nil {
			errs = append(errs, *err)
		}

		// Pattern validation
		if err := s.validatePattern(path, value, fieldSchema); err != nil {
			errs = append(errs, *err)
		}

		// Length validation
		if err := s.validateLength(path, value, fieldSchema); err != nil {
			errs = append(errs, *err)
		}
	}

	return errs
}

// validateType checks if the value matches the expected type.
func (s *Schema) validateType(path string, value any, fs FieldSchema) *ValidationError {
	valid := false

	switch fs.Type {
	case TypeString, TypeDuration, TypeSize:
		_, valid = value.(string)
	case TypeInt:
		switch value.(type) {
		case int, int64, float64:
			valid = true
		}
	case TypeFloat:
		switch value.(type) {
		case float64, float32, int:
			valid = true
		}
	case TypeBool:
		_, valid = value.(bool)
	case TypeArray:
		kind := reflect.TypeOf(value).Kind()
		valid = kind == reflect.Slice || kind == reflect.Array
	case TypeMap:
		kind := reflect.TypeOf(value).Kind()
		valid = kind == reflect.Map
	}

	if !valid {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected %s type", fs.Type),
			Value:   value,
		}
	}
	return nil
}

// validateRange checks numeric range constraints.
func (s *Schema) validateRange(path string, value any, fs FieldSchema) *ValidationError {
	var num float64

	switch v := value.(type) {
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case float64:
		num = v
	default:
		return nil
	}

	if fs.Min != nil && num < *fs.Min {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("value must be >= %v", *fs.Min),
			Value:   value,
		}
	}

	if fs.Max != nil && num > *fs.Max {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("value must be <= %v", *fs.Max),
			Value:   value,
		}
	}

	return nil
}

// validateEnum checks if value is in allowed enum values.
func (s *Schema) validateEnum(path string, value any, fs FieldSchema) *ValidationError {
	if len(fs.Enum) == 0 {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return nil
	}

	for _, allowed := range fs.Enum {
		if str == allowed {
			return nil
		}
	}

	return &ValidationError{
		Field:   path,
		Message: fmt.Sprintf("must be one of: %v", fs.Enum),
		Value:   value,
	}
}

// validatePattern checks regex pattern constraint.
func (s *Schema) validatePattern(path string, value any, fs FieldSchema) *ValidationError {
	if fs.Pattern == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return nil
	}

	if !fs.Pattern.MatchString(str) {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("must match pattern: %s", fs.Pattern.String()),
			Value:   value,
		}
	}

	return nil
}

// validateLength checks string/array length constraints.
func (s *Schema) validateLength(path string, value any, fs FieldSchema) *ValidationError {
	var length int

	switch v := value.(type) {
	case string:
		length = len(v)
	case []any:
		length = len(v)
	default:
		return nil
	}

	if fs.MinLen != nil && length < *fs.MinLen {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("length must be >= %d", *fs.MinLen),
			Value:   value,
		}
	}

	if fs.MaxLen != nil && length > *fs.MaxLen {
		return &ValidationError{
			Field:   path,
			Message: fmt.Sprintf("length must be <= %d", *fs.MaxLen),
			Value:   value,
		}
	}

	return nil
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return nil
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			return val
		}

		// Otherwise, descend into nested map
		next, ok := val.(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}

	return nil
}

// GenerateSchemaDoc generates documentation from the schema.
func (s *Schema) GenerateSchemaDoc() string {
	var b strings.Builder

	b.WriteString("# Automergent Configuration Schema\n\n")

	// Group by prefix
	groups := make(map[string][]string)
	for path := range s.Fields {
		parts := strings.SplitN(path, ".", 2)
		group := "General"
		if len(parts) > 1 {
			group = strings.Title(parts[0])
		}
		groups[group] = append(groups[group], path)
	}

	for group, paths := range groups {
		b.WriteString(fmt.Sprintf("## %s\n\n", group))

		for _, path := range paths {
			fs := s.Fields[path]
			b.WriteString(fmt.Sprintf("### `%s`\n\n", path))

			if fs.Description != "" {
				b.WriteString(fmt.Sprintf("%s\n\n", fs.Description))
			}

			b.WriteString(fmt.Sprintf("- **Type**: %s\n", fs.Type))

			if fs.Default != nil {
				b.WriteString(fmt.Sprintf("- **Default**: `%v`\n", fs.Default))
			}

			if fs.Required {
				b.WriteString("- **Required**: yes\n")
			}

			if len(fs.Enum) > 0 {
				b.WriteString(fmt.Sprintf("- **Allowed values**: %v\n", fs.Enum))
			}

			if fs.Min != nil || fs.Max != nil {
				if fs.Min != nil && fs.Max != nil {
					b.WriteString(fmt.Sprintf("- **Range**: %v - %v\n", *fs.Min, *fs.Max))
				} else if fs.Min != nil {
					b.WriteString(fmt.Sprintf("- **Minimum**: %v\n", *fs.Min))
				} else {
					b.WriteString(fmt.Sprintf("- **Maximum**: %v\n", *fs.Max))
				}
			}

			if fs.Deprecated {
				b.WriteString("- **Deprecated**: yes\n")
			}

			if fs.Sensitive {
				b.WriteString("- **Sensitive**: yes (secret)\n")
			}

			b.WriteString("\n")
		}
	}

	return b.String()
}

// ValidateFile validates a config file without loading it.
func ValidateFile(path string) ([]ValidationError, error) {
	loader, err := NewLoader(&LoaderOptions{
		GlobalPath: path,
		Schema:     DefaultSchema(),
	})
	if err != nil {
		return nil, err
	}

	_, err = loader.Load()
	if err != nil {
		// Try to extract validation errors
		return []ValidationError{{Message: err.Error()}}, nil
	}

	return nil, nil
}
