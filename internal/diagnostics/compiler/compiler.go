// Package compiler provides multi-language compiler output parsing.
// It parses error messages from various compilers and linters to produce
// standardized diagnostic information.
package compiler

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// Language represents a programming language.
type Language = types.Language

// Supported languages for compiler output parsing.
const (
	LangGo         = types.LangGo
	LangTypeScript = types.LangTypeScript
	LangJavaScript = types.LangJavaScript
	LangPython     = types.LangPython
	LangRust       = types.LangRust
	LangJava       = types.LangJava
	LangCPP        = types.LangCPP
	LangC          = types.LangC
	LangCSharp     = types.LangCSharp
	LangRuby       = types.LangRuby
	LangPHP        = types.LangPHP
	LangGeneric    Language = "generic"
)

// ErrorCategory classifies the type of error.
type ErrorCategory string

// Error categories for classification.
const (
	CategorySyntax      ErrorCategory = "syntax"
	CategoryType        ErrorCategory = "type"
	CategoryImport      ErrorCategory = "import"
	CategoryRuntime     ErrorCategory = "runtime"
	CategoryLogic       ErrorCategory = "logic"
	CategoryDependency  ErrorCategory = "dependency"
	CategoryConfig      ErrorCategory = "config"
	CategoryDeprecation ErrorCategory = "deprecation"
	CategorySecurity    ErrorCategory = "security"
	CategoryPerformance ErrorCategory = "performance"
	CategoryStyle       ErrorCategory = "style"
	CategoryUnknown     ErrorCategory = "unknown"
)

// CompilerDiagnostic extends Diagnostic with compiler-specific information.
// Note: range/suggestion/tag fields come from the embedded types.Diagnostic —
// redeclaring them here would shadow the embedded ones and silently drop the
// values whenever a CompilerDiagnostic is converted to a plain Diagnostic.
type CompilerDiagnostic struct {
	types.Diagnostic
	Category ErrorCategory `json:"category"`
	Compiler string        `json:"compiler"` // e.g., "go", "tsc", "rustc"
	ErrorCode string       `json:"error_code"` // e.g., "E0382" for Rust
	Context  []string      `json:"context"`   // Additional context lines
	URL      string        `json:"url,omitempty"` // Documentation URL
}

// Parser interface for language-specific parsers.
type Parser interface {
	Parse(output string) []CompilerDiagnostic
	Language() Language
}

// ParserRegistry holds all registered parsers.
type ParserRegistry struct {
	parsers map[Language]Parser
}

// NewParserRegistry creates a new parser registry with all built-in parsers.
func NewParserRegistry() *ParserRegistry {
	r := &ParserRegistry{
		parsers: make(map[Language]Parser),
	}

	// Register all parsers
	r.Register(&GoParser{})
	r.Register(&TypeScriptParser{})
	r.Register(&PythonParser{})
	r.Register(&RustParser{})
	r.Register(&JavaParser{})
	r.Register(&GenericParser{})

	return r
}

// Register adds a parser to the registry.
func (r *ParserRegistry) Register(p Parser) {
	r.parsers[p.Language()] = p
}

// Get returns the parser for a language.
func (r *ParserRegistry) Get(lang Language) Parser {
	if p, ok := r.parsers[lang]; ok {
		return p
	}
	return r.parsers[LangGeneric]
}

// Parse parses compiler output for the specified language.
func (r *ParserRegistry) Parse(lang Language, output string) []CompilerDiagnostic {
	return r.Get(lang).Parse(output)
}

// DetectLanguage attempts to detect the language from compiler output.
func DetectLanguage(output string) Language {
	output = strings.ToLower(output)

	switch {
	case strings.Contains(output, "go build") || strings.Contains(output, "go test") ||
		strings.Contains(output, "cannot find package") || strings.Contains(output, "undefined:"):
		return LangGo
	case strings.Contains(output, "tsc") || strings.Contains(output, "ts(") ||
		strings.Contains(output, "error ts"):
		return LangTypeScript
	case strings.Contains(output, "eslint") || strings.Contains(output, "jshint"):
		return LangJavaScript
	case strings.Contains(output, "mypy:") || strings.Contains(output, "pylint:") ||
		strings.Contains(output, "syntaxerror:") || strings.Contains(output, "traceback"):
		return LangPython
	case strings.Contains(output, "error[e") || strings.Contains(output, "cargo"):
		return LangRust
	case strings.Contains(output, "javac") || strings.Contains(output, "java.lang"):
		return LangJava
	case strings.Contains(output, "gcc") || strings.Contains(output, "g++") ||
		strings.Contains(output, "clang"):
		return LangCPP
	default:
		return LangGeneric
	}
}

// ─── Go Parser ───────────────────────────────────────────────────────────────

// GoParser parses Go compiler and tool output.
type GoParser struct{}

// Language returns the language identifier.
func (p *GoParser) Language() Language { return LangGo }

// Parse parses Go compiler output.
func (p *GoParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// Go compiler pattern: file.go:line:col: message
	goPattern := regexp.MustCompile(`([^:\s]+\.go):(\d+):(\d+):\s*(.+)`)
	// Go vet/lint pattern: file.go:line: message
	vetPattern := regexp.MustCompile(`([^:\s]+\.go):(\d+):\s*(.+)`)
	// Package import error
	importPattern := regexp.MustCompile(`cannot find package "([^"]+)"`)
	// Undefined variable
	undefinedPattern := regexp.MustCompile(`undefined:\s*(\w+)`)
	// Type mismatch
	typePattern := regexp.MustCompile(`cannot use .+ \(type ([^)]+)\) as type ([^)\s]+)`)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var diag CompilerDiagnostic
		diag.Compiler = "go"
		diag.Source = "go-compiler"

		matched := false
		if matches := goPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Message = matches[4]
			diag.Severity = "error"
			diag.Category = categorizeGoError(matches[4])
			diag.Tags = []string{"go", "compile"}
			diag.Suggestions = []string{"Fix the reported error and rebuild", "Run 'go vet' for additional checks"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			matched = true
		} else if matches := vetPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Message = matches[3]
			diag.Severity = "warning"
			diag.Category = CategoryStyle
			diag.Tags = []string{"go", "vet", "style"}
			diag.Suggestions = []string{"Address the vet warning", "Run 'go vet ./...' for full analysis"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			matched = true
		}
		if !matched {
			continue
		}

		// Enrichment patterns only apply to the diagnostic produced by this
		// line — attaching them to an earlier, unrelated diagnostic
		// miscategorizes it.
		if importPattern.MatchString(line) {
			diag.Category = CategoryImport
			diag.Tags = append(diag.Tags, "import")
			diag.Suggestions = []string{"Run 'go mod tidy' to download missing dependencies", "Check import path spelling", "Verify module is available in registry"}
		}
		if undefinedPattern.MatchString(line) {
			diag.Category = CategoryType
			diag.Tags = append(diag.Tags, "undefined")
			diag.Suggestions = []string{"Check variable/function name spelling", "Ensure identifier is declared in scope", "Check for missing imports"}
		}
		if typePattern.MatchString(line) {
			diag.Category = CategoryType
			diag.Tags = append(diag.Tags, "type-mismatch")
			diag.Suggestions = []string{"Fix type conversion", "Check function signature matches", "Verify generic type parameters"}
		}
		diags = append(diags, diag)
	}

	return diags
}

func categorizeGoError(msg string) ErrorCategory {
	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "syntax error") || strings.Contains(msg, "unexpected"):
		return CategorySyntax
	case strings.Contains(msg, "undefined") || strings.Contains(msg, "cannot use"):
		return CategoryType
	case strings.Contains(msg, "import") || strings.Contains(msg, "package"):
		return CategoryImport
	case strings.Contains(msg, "deprecated"):
		return CategoryDeprecation
	default:
		return CategoryUnknown
	}
}

// ─── TypeScript Parser ───────────────────────────────────────────────────────

// TypeScriptParser parses TypeScript compiler and ESLint output.
type TypeScriptParser struct{}

// Language returns the language identifier.
func (p *TypeScriptParser) Language() Language { return LangTypeScript }

// Parse parses TypeScript compiler output.
func (p *TypeScriptParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// TSC pattern: file.ts(line,col): error TSxxxx: message
	tscPattern := regexp.MustCompile(`([^(\s]+)\((\d+),(\d+)\):\s*(error|warning)\s*(TS\d+):\s*(.+)`)
	// ESLint pattern: file.ts:line:col: severity rule message
	eslintPattern := regexp.MustCompile(`([^:\s]+):(\d+):(\d+):\s*(error|warning)\s+(.+)`)
	// TSC alternative: file.ts:line:col - error TSxxxx: message
	tscAltPattern := regexp.MustCompile(`([^:\s]+):(\d+):(\d+)\s*-\s*(error|warning)\s*(TS\d+):\s*(.+)`)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var diag CompilerDiagnostic
		diag.Compiler = "tsc"
		diag.Source = "typescript"

		if matches := tscPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Severity = matches[4]
			diag.ErrorCode = matches[5]
			diag.Message = matches[6]
			diag.Category = categorizeTSError(matches[5], matches[6])
			diag.URL = "https://www.typescriptlang.org/docs/handbook/2/errors.html#" + strings.ToLower(matches[5])
			diag.Tags = []string{"typescript", "compile"}
			diag.Suggestions = []string{"Fix the TypeScript error", "Run 'tsc --noEmit' for full check"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		} else if matches := tscAltPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Severity = matches[4]
			diag.ErrorCode = matches[5]
			diag.Message = matches[6]
			diag.Category = categorizeTSError(matches[5], matches[6])
			diag.Tags = []string{"typescript", "compile"}
			diag.Suggestions = []string{"Fix the TypeScript error", "Run 'tsc --noEmit' for full check"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		} else if matches := eslintPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Severity = matches[4]
			diag.Message = matches[5]
			diag.Compiler = "eslint"
			diag.Source = "eslint"
			diag.Category = CategoryStyle
			diag.Tags = []string{"typescript", "lint", "style"}
			diag.Suggestions = []string{"Run 'eslint --fix' to auto-fix", "Check eslint rule configuration"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		}
	}

	return diags
}

func categorizeTSError(code, msg string) ErrorCategory {
	// TypeScript error code ranges
	codeNum, _ := strconv.Atoi(strings.TrimPrefix(code, "TS"))
	switch {
	case codeNum >= 1000 && codeNum < 2000:
		return CategorySyntax
	case codeNum >= 2000 && codeNum < 3000:
		return CategoryType
	case codeNum >= 4000 && codeNum < 5000:
		return CategoryImport
	default:
		msg = strings.ToLower(msg)
		if strings.Contains(msg, "import") || strings.Contains(msg, "module") {
			return CategoryImport
		}
		if strings.Contains(msg, "type") || strings.Contains(msg, "assignable") {
			return CategoryType
		}
		return CategoryUnknown
	}
}

// ─── Python Parser ───────────────────────────────────────────────────────────

// PythonParser parses Python error messages from various tools.
type PythonParser struct{}

// Language returns the language identifier.
func (p *PythonParser) Language() Language { return LangPython }

// Parse parses Python compiler/linter output.
func (p *PythonParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// Python traceback pattern
	filePattern := regexp.MustCompile(`File "([^"]+)", line (\d+)`)
	// Syntax error pattern
	syntaxPattern := regexp.MustCompile(`SyntaxError:\s*(.+)`)
	// Mypy pattern: file.py:line: severity: message
	mypyPattern := regexp.MustCompile(`([^:\s]+\.py):(\d+):\s*(error|warning|note):\s*(.+)`)
	// Pylint pattern: file.py:line:col: code: message
	pylintPattern := regexp.MustCompile(`([^:\s]+\.py):(\d+):(\d+):\s*([A-Z]\d+):\s*(.+)`)
	// Flake8 pattern: file.py:line:col: code message
	flake8Pattern := regexp.MustCompile(`([^:\s]+\.py):(\d+):(\d+):\s*([A-Z]\d+)\s+(.+)`)

	lines := strings.Split(output, "\n")
	var currentFile string
	var currentLine int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var diag CompilerDiagnostic
		diag.Compiler = "python"
		diag.Source = "python"

		if matches := mypyPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Severity = matches[3]
			diag.Message = matches[4]
			diag.Compiler = "mypy"
			diag.Source = "mypy"
			diag.Category = categorizePythonError(matches[4])
			diag.Tags = []string{"python", "type-check"}
			diag.Suggestions = []string{"Run 'mypy --strict' for full analysis", "Add type annotations to fix errors"}
			diag.EndLine = diag.Line
			diag.EndColumn = 1
			diags = append(diags, diag)
		} else if matches := pylintPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.ErrorCode = matches[4]
			diag.Message = matches[5]
			diag.Compiler = "pylint"
			diag.Source = "pylint"
			diag.Severity = categorizePylintCode(matches[4])
			diag.Category = CategoryStyle
			diag.Tags = []string{"python", "lint", "style"}
			diag.Suggestions = []string{"Run 'pylint --fix' for auto-fixes", "Check pylint configuration"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		} else if matches := flake8Pattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.ErrorCode = matches[4]
			diag.Message = matches[5]
			diag.Compiler = "flake8"
			diag.Source = "flake8"
			diag.Severity = "warning"
			diag.Category = CategoryStyle
			diag.Tags = []string{"python", "lint", "style"}
			diag.Suggestions = []string{"Run 'flake8 --select' to filter", "Configure flake8 rules"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		} else if matches := filePattern.FindStringSubmatch(line); matches != nil {
			currentFile = matches[1]
			currentLine, _ = strconv.Atoi(matches[2])
		} else if matches := syntaxPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = currentFile
			diag.Line = currentLine
			diag.Message = "SyntaxError: " + matches[1]
			diag.Severity = "error"
			diag.Category = CategorySyntax
			diag.Tags = []string{"python", "syntax"}
			diag.Suggestions = []string{"Check for missing colons, brackets, or indentation", "Run 'python -m py_compile' to validate"}
			diag.EndLine = diag.Line
			diag.EndColumn = 1
			diags = append(diags, diag)
		}
	}

	return diags
}

func categorizePythonError(msg string) ErrorCategory {
	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "syntax"):
		return CategorySyntax
	case strings.Contains(msg, "type") || strings.Contains(msg, "incompatible"):
		return CategoryType
	case strings.Contains(msg, "import") || strings.Contains(msg, "module"):
		return CategoryImport
	case strings.Contains(msg, "undefined") || strings.Contains(msg, "name"):
		return CategoryType
	default:
		return CategoryUnknown
	}
}

func categorizePylintCode(code string) string {
	if len(code) == 0 {
		return "warning"
	}
	switch code[0] {
	case 'E', 'F':
		return "error"
	case 'W':
		return "warning"
	case 'C', 'R':
		return "info"
	default:
		return "warning"
	}
}

// ─── Rust Parser ─────────────────────────────────────────────────────────────

// RustParser parses Rust compiler (rustc/cargo) output.
type RustParser struct{}

// Language returns the language identifier.
func (p *RustParser) Language() Language { return LangRust }

// Parse parses Rust compiler output.
func (p *RustParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// Rust error pattern: error[E0xxx]: message
	errorHeaderPattern := regexp.MustCompile(`(error|warning)(\[E\d+\])?:\s*(.+)`)
	// Location pattern: --> file.rs:line:col
	locationPattern := regexp.MustCompile(`-->\s*([^:\s]+):(\d+):(\d+)`)
	// Help pattern: help: message
	helpPattern := regexp.MustCompile(`help:\s*(.+)`)
	// Note pattern: note: message
	notePattern := regexp.MustCompile(`note:\s*(.+)`)

	lines := strings.Split(output, "\n")
	var currentDiag *CompilerDiagnostic

	for _, line := range lines {
		if matches := errorHeaderPattern.FindStringSubmatch(line); matches != nil {
			if currentDiag != nil && currentDiag.FilePath != "" {
				diags = append(diags, *currentDiag)
			}
			currentDiag = &CompilerDiagnostic{
				Compiler: "rustc",
			}
			currentDiag.Source = "rustc"
			currentDiag.Severity = matches[1]
			if matches[2] != "" {
				currentDiag.ErrorCode = strings.Trim(matches[2], "[]")
				currentDiag.URL = "https://doc.rust-lang.org/error_codes/" + currentDiag.ErrorCode + ".html"
			}
			currentDiag.Message = matches[3]
			currentDiag.Category = categorizeRustError(currentDiag.ErrorCode, currentDiag.Message)
			currentDiag.Tags = []string{"rust", "compile"}
			currentDiag.Suggestions = []string{"Run 'cargo check' for full analysis", "Check Rust compiler error documentation"}
		} else if matches := locationPattern.FindStringSubmatch(line); matches != nil && currentDiag != nil {
			currentDiag.FilePath = matches[1]
			currentDiag.Line, _ = strconv.Atoi(matches[2])
			currentDiag.Column, _ = strconv.Atoi(matches[3])
			currentDiag.EndLine = currentDiag.Line
			currentDiag.EndColumn = currentDiag.Column + 1
		} else if matches := helpPattern.FindStringSubmatch(line); matches != nil && currentDiag != nil {
			currentDiag.Suggestions = append(currentDiag.Suggestions, matches[1])
		} else if matches := notePattern.FindStringSubmatch(line); matches != nil && currentDiag != nil {
			currentDiag.Context = append(currentDiag.Context, matches[1])
		}
	}

	if currentDiag != nil && currentDiag.FilePath != "" {
		diags = append(diags, *currentDiag)
	}

	return diags
}

func categorizeRustError(code, msg string) ErrorCategory {
	// Rust error code ranges
	if code != "" {
		codeNum, _ := strconv.Atoi(strings.TrimPrefix(code, "E"))
		switch {
		case codeNum >= 1 && codeNum < 100:
			return CategorySyntax
		case codeNum >= 100 && codeNum < 300:
			return CategoryType
		case codeNum >= 400 && codeNum < 500:
			return CategoryImport
		}
	}

	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "borrow") || strings.Contains(msg, "lifetime"):
		return CategoryType
	case strings.Contains(msg, "import") || strings.Contains(msg, "unresolved"):
		return CategoryImport
	case strings.Contains(msg, "syntax") || strings.Contains(msg, "unexpected"):
		return CategorySyntax
	default:
		return CategoryUnknown
	}
}

// ─── Java Parser ─────────────────────────────────────────────────────────────

// JavaParser parses Java compiler (javac) output.
type JavaParser struct{}

// Language returns the language identifier.
func (p *JavaParser) Language() Language { return LangJava }

// Parse parses Java compiler output.
func (p *JavaParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// javac pattern: file.java:line: error/warning: message
	javacPattern := regexp.MustCompile(`([^:\s]+\.java):(\d+):\s*(error|warning):\s*(.+)`)
	// Maven/Gradle pattern with column: [ERROR] file.java:[line,col] message
	mavenPattern := regexp.MustCompile(`\[(ERROR|WARNING)\]\s*([^:\s]+\.java):\[(\d+),(\d+)\]\s*(.+)`)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var diag CompilerDiagnostic
		diag.Compiler = "javac"
		diag.Source = "javac"

		if matches := javacPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Severity = matches[3]
			diag.Message = matches[4]
			diag.Category = categorizeJavaError(matches[4])
			diag.Tags = []string{"java", "compile"}
			diag.Suggestions = []string{"Fix the Java compiler error", "Run 'mvn compile' or 'gradle build' for full check"}
			diag.EndLine = diag.Line
			diag.EndColumn = 1
			diags = append(diags, diag)
		} else if matches := mavenPattern.FindStringSubmatch(line); matches != nil {
			diag.FilePath = matches[2]
			diag.Line, _ = strconv.Atoi(matches[3])
			diag.Column, _ = strconv.Atoi(matches[4])
			diag.Severity = strings.ToLower(matches[1])
			diag.Message = matches[5]
			diag.Category = categorizeJavaError(matches[5])
			diag.Tags = []string{"java", "compile", "maven"}
			diag.Suggestions = []string{"Fix the Java compiler error", "Run 'mvn compile' or 'gradle build' for full check"}
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
			diags = append(diags, diag)
		}
	}

	return diags
}

func categorizeJavaError(msg string) ErrorCategory {
	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "cannot find symbol") || strings.Contains(msg, "symbol"):
		return CategoryType
	case strings.Contains(msg, "package") || strings.Contains(msg, "import"):
		return CategoryImport
	case strings.Contains(msg, "incompatible types"):
		return CategoryType
	case strings.Contains(msg, "expected") || strings.Contains(msg, "';' expected"):
		return CategorySyntax
	default:
		return CategoryUnknown
	}
}

// ─── Generic Parser ──────────────────────────────────────────────────────────

// GenericParser handles generic compiler output patterns.
type GenericParser struct{}

// Language returns the language identifier.
func (p *GenericParser) Language() Language { return LangGeneric }

// Generic output patterns, tried in order per line.
var (
	fileLineCol     = regexp.MustCompile(`([^:\s]+):(\d+):(\d+):\s*(error|warning|info)?:?\s*(.+)`)
	fileLine        = regexp.MustCompile(`([^:\s]+):(\d+):\s*(error|warning|info)?:?\s*(.+)`)
	fileLineColParen = regexp.MustCompile(`([^(\s]+)\((\d+),(\d+)\):\s*(error|warning|info)?:?\s*(.+)`)
	bracketed       = regexp.MustCompile(`\[(ERROR|WARNING|INFO)\]\s*([^:\s]+):(\d+)\s*[-:]\s*(.+)`)
)

// Parse parses generic compiler output.
func (p *GenericParser) Parse(output string) []CompilerDiagnostic {
	var diags []CompilerDiagnostic

	// Common patterns:
	// file:line:col: message
	// file:line: message
	// file(line,col): message
	// [ERROR] file:line - message

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var diag CompilerDiagnostic
		diag.Compiler = "generic"
		diag.Source = "compiler"
		diag.Category = CategoryUnknown
		diag.Tags = []string{"generic"}
		diag.Suggestions = []string{"Review the error message and fix the issue"}

		matched := true
		if matches := fileLineCol.FindStringSubmatch(line); matches != nil {
			// file:line:col: [severity:] message
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Severity = normalizeSeverity(matches[4], "error")
			diag.Message = matches[5]
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
		} else if matches := fileLineColParen.FindStringSubmatch(line); matches != nil {
			// file(line,col): [severity:] message
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Column, _ = strconv.Atoi(matches[3])
			diag.Severity = normalizeSeverity(matches[4], "error")
			diag.Message = matches[5]
			diag.EndLine = diag.Line
			diag.EndColumn = diag.Column + 1
		} else if matches := fileLine.FindStringSubmatch(line); matches != nil {
			// file:line: [severity:] message
			diag.FilePath = matches[1]
			diag.Line, _ = strconv.Atoi(matches[2])
			diag.Severity = normalizeSeverity(matches[3], "error")
			diag.Message = matches[4]
			diag.EndLine = diag.Line
			diag.EndColumn = 1
		} else if matches := bracketed.FindStringSubmatch(line); matches != nil {
			// [ERROR] file:line - message — the severity is the bracketed
			// word, not a path component.
			diag.FilePath = matches[2]
			diag.Line, _ = strconv.Atoi(matches[3])
			diag.Severity = strings.ToLower(matches[1])
			diag.Message = matches[4]
			diag.EndLine = diag.Line
			diag.EndColumn = 1
		} else {
			matched = false
		}

		if !matched {
			continue
		}
		diag.Category = categorizeGenericError(diag.Message)
		diags = append(diags, diag)
	}

	return diags
}

// normalizeSeverity maps an optional severity word to a canonical lowercase
// severity, falling back to def when absent or unrecognized.
func normalizeSeverity(s, def string) string {
	switch strings.ToLower(s) {
	case "error", "warning", "info", "hint":
		return strings.ToLower(s)
	}
	return def
}

func categorizeGenericError(msg string) ErrorCategory {
	msg = strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "syntax"):
		return CategorySyntax
	case strings.Contains(msg, "type") || strings.Contains(msg, "undefined"):
		return CategoryType
	case strings.Contains(msg, "import") || strings.Contains(msg, "module") || strings.Contains(msg, "package"):
		return CategoryImport
	case strings.Contains(msg, "runtime") || strings.Contains(msg, "panic"):
		return CategoryRuntime
	case strings.Contains(msg, "deprecated"):
		return CategoryDeprecation
	case strings.Contains(msg, "security") || strings.Contains(msg, "vulnerability"):
		return CategorySecurity
	default:
		return CategoryUnknown
	}
}

// ParseOutput parses compiler output and returns diagnostics.
// It auto-detects the language if not specified.
func ParseOutput(output string, lang Language) []CompilerDiagnostic {
	registry := NewParserRegistry()
	if lang == "" {
		lang = DetectLanguage(output)
	}
	return registry.Parse(lang, output)
}
