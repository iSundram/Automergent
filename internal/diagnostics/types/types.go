// Package types defines shared types for the diagnostics system.
package types

import (
	"fmt"
	"strconv"
)

// Diagnostic represents a single code diagnostic (error or warning).
type Diagnostic struct {
	FilePath string `json:"file_path,omitempty"`
	Line     int    `json:"line"`     // 1-indexed
	Column   int    `json:"column"`   // 0-indexed
	Severity string `json:"severity"` // "error" | "warning"
	Code     string `json:"code"`     // e.g. "syntax-error", "missing-brace"
	Message  string `json:"message"`  // human-readable description
	Source   string `json:"source"`   // e.g. "tree-sitter-go"
}

// Key returns a string that uniquely identifies a diagnostic by position and
// code – used for delta comparison.
func (d Diagnostic) Key() string {
	return fmt.Sprintf("%s@%s:%s:%s", d.Code, d.Source, strconv.Itoa(d.Line), strconv.Itoa(d.Column))
}
