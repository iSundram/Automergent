package tools

import (
	"encoding/json"
)

// StringArg returns (s, true) if key exists and the value is a string (may be empty).
func StringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// ArgFloat returns a numeric argument as float64 (JSON numbers are often float64).
func ArgFloat(args map[string]any, key string) (float64, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// ArgInt returns a non-negative integer argument.
func ArgInt(args map[string]any, key string) (int, bool) {
	f, ok := ArgFloat(args, key)
	if !ok {
		return 0, false
	}
	n := int(f)
	if float64(n) != f {
		return 0, false
	}
	return n, true
}

// ArgBool returns a boolean argument.
func ArgBool(args map[string]any, key string) (bool, bool) {
	v, ok := args[key]
	if !ok {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch x {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		}
	case float64:
		return x != 0, true
	case int:
		return x != 0, true
	case int64:
		return x != 0, true
	}
	return false, false
}
