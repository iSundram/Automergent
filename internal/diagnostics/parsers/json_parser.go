// json_parser.go provides a pure-Go JSON syntax checker that does not require
// a tree-sitter grammar.
package parsers

import (
	"encoding/json"
	"fmt"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// ParseJSON validates JSON content and returns diagnostics.
func ParseJSON(content []byte) []types.Diagnostic {
	var dummy interface{}
	if err := json.Unmarshal(content, &dummy); err != nil {
		// json.SyntaxError carries a byte offset.
		var se *json.SyntaxError
		line, col := offsetToLineCol(content, 0)
		msg := err.Error()
		if jsonErr, ok := err.(*json.SyntaxError); ok {
			se = jsonErr
			line, col = offsetToLineCol(content, int(se.Offset-1))
		}
		return []types.Diagnostic{
			{
				Line:     line,
				Column:   col,
				Severity: "error",
				Code:     "json-syntax-error",
				Message:  fmt.Sprintf("JSON syntax error: %s", msg),
				Source:   "json-parser",
			},
		}
	}
	return nil
}

// offsetToLineCol converts a byte offset in src to a 1-indexed line and
// 0-indexed column number.
func offsetToLineCol(src []byte, offset int) (line, col int) {
	line = 1
	col = 0
	if offset >= len(src) {
		offset = len(src) - 1
	}
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}
