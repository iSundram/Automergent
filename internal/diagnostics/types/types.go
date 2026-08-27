// Package types defines shared types for the diagnostics system.
package types

import (
	"fmt"
	"strings"
)

// Language represents a programming language identifier.
type Language string

// Supported language identifiers.
const (
	LangGo         Language = "go"
	LangPython     Language = "python"
	LangJavaScript Language = "javascript"
	LangJSX        Language = "jsx"
	LangTypeScript Language = "typescript"
	LangTSX        Language = "tsx"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangC          Language = "c"
	LangCPP        Language = "cpp"
	LangCSharp     Language = "csharp"
	LangRuby       Language = "ruby"
	LangPHP        Language = "php"
	LangSwift      Language = "swift"
	LangKotlin     Language = "kotlin"
	LangLua        Language = "lua"
	LangJSON       Language = "json"
	LangYAML       Language = "yaml"
	LangTOML       Language = "toml"
)

// Diagnostic represents a single code diagnostic (error or warning).
type Diagnostic struct {
	FilePath     string   `json:"file_path,omitempty"`
	Line         int      `json:"line"`          // 1-indexed start line
	Column       int      `json:"column"`        // 0-indexed start column
	EndLine      int      `json:"end_line,omitempty"`     // 1-indexed end line (inclusive)
	EndColumn    int      `json:"end_column,omitempty"`   // 0-indexed end column (exclusive)
	Severity     string   `json:"severity"`      // "error" | "warning" | "info" | "hint"
	Code         string   `json:"code"`          // e.g. "syntax-error", "unused-variable"
	Message      string   `json:"message"`       // human-readable description
	Source       string   `json:"source"`        // e.g. "tree-sitter-go", "go vet", "eslint"
	Tags         []string `json:"tags,omitempty"`        // e.g. "unused", "deprecated", "security"
	Suggestions  []string `json:"suggestions,omitempty"` // Quick-fix actions
	RelatedFiles []string `json:"related_files,omitempty"` // For cross-file error chains
}

// Key returns a string that uniquely identifies a diagnostic by position and
// code – used for delta comparison.
func (d Diagnostic) Key() string {
	end := ""
	if d.EndLine > 0 || d.EndColumn > 0 {
		end = fmt.Sprintf("-%d:%d", d.EndLine, d.EndColumn)
	}
	return fmt.Sprintf("%s@%s:%d:%d%s", d.Code, d.Source, d.Line, d.Column, end)
}

// Range returns the diagnostic as a range (start to end).
func (d Diagnostic) Range() DiagnosticRange {
	return DiagnosticRange{
		Start: Position{Line: d.Line, Column: d.Column},
		End:   Position{Line: d.EndLine, Column: d.EndColumn},
	}
}

// HasTag checks if the diagnostic has a specific tag.
func (d Diagnostic) HasTag(tag string) bool {
	for _, t := range d.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// DiagnosticRange represents a source code range (inclusive start, exclusive end).
type DiagnosticRange struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a 1-indexed line, 0-indexed column position.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// IsZero returns true if the position is unset.
func (p Position) IsZero() bool {
	return p.Line == 0 && p.Column == 0
}

// IsSingleLine returns true if the range is on a single line.
func (r DiagnosticRange) IsSingleLine() bool {
	return r.Start.Line == r.End.Line || r.End.Line == 0
}

// Contains checks if a position falls within the range.
func (r DiagnosticRange) Contains(pos Position) bool {
	if pos.Line < r.Start.Line || (pos.Line == r.Start.Line && pos.Column < r.Start.Column) {
		return false
	}
	if r.End.IsZero() {
		return true
	}
	if pos.Line > r.End.Line || (pos.Line == r.End.Line && pos.Column >= r.End.Column) {
		return false
	}
	return true
}

// String returns a human-readable range string.
func (r DiagnosticRange) String() string {
	if r.End.IsZero() || r.IsSingleLine() {
		return fmt.Sprintf("%d:%d", r.Start.Line, r.Start.Column)
	}
	return fmt.Sprintf("%d:%d-%d:%d", r.Start.Line, r.Start.Column, r.End.Line, r.End.Column)
}

// WithDefaults fills in missing fields with sensible defaults.
func (d *Diagnostic) WithDefaults() {
	if d.Severity == "" {
		d.Severity = "error"
	}
	if d.EndLine == 0 {
		d.EndLine = d.Line
	}
	if d.EndColumn == 0 && d.EndLine == d.Line {
		d.EndColumn = d.Column + 1
	}
}
