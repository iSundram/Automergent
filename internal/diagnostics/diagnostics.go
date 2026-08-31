// Package diagnostics provides the core error-detection engine for ORTEDS.
//
// Usage:
//
//	diags := diagnostics.Analyze("main.go", content)
//	delta := diagnostics.Compare(before, after)
package diagnostics

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	diagcache "github.com/iSundram/Automergent/internal/diagnostics/cache"
	"github.com/iSundram/Automergent/internal/diagnostics/compiler"
	"github.com/iSundram/Automergent/internal/diagnostics/linters"
	"github.com/iSundram/Automergent/internal/diagnostics/parsers"
	"github.com/iSundram/Automergent/internal/diagnostics/recovery"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

var linterRegistry = linters.NewLinterRegistry()

// Diagnostic is re-exported from types for convenience.
type Diagnostic = types.Diagnostic

// RecoveryReport summarizes diagnostics into actionable recovery guidance.
type RecoveryReport = recovery.Report

// Analyze parses content as the language inferred from path and returns all
// detected diagnostics.  It returns nil when the language is unsupported or
// when the file exceeds the configured size limit.
//
// Analyze is subprocess-free: it runs tree-sitter syntax and semantic checks
// only, so it is safe on hot paths (every read_file and edit_file call).
// External linters are available separately via Lint.
func Analyze(path, content string) []Diagnostic {
	cfg := loadConfig()
	if !cfg.Diagnostics.Enabled {
		return nil
	}
	if int64(len(content)) > cfg.Diagnostics.MaxFileSizeBytes {
		return nil
	}

	lang := DetectLanguage(path)
	if lang == "" {
		return nil
	}

	return analyzeWithCache(path, lang, content)
}

// Lint runs external linters (golangci-lint, ruff, …) for the language
// inferred from path. Unlike Analyze it shells out and can take seconds, so
// it is meant for explicit checks (the /lsp command, lsp_diagnostics tool),
// not the read/edit hot path.
func Lint(ctx context.Context, path, content string) []Diagnostic {
	cfg := loadConfig()
	if !cfg.Diagnostics.Enabled {
		return nil
	}
	lang := DetectLanguage(path)
	if lang == "" {
		return nil
	}
	diags, err := linterRegistry.Lint(ctx, lang, path, content)
	if err != nil {
		return nil
	}
	return diags
}

func analyzeWithCache(path string, lang parsers.Language, content string) []Diagnostic {
	// Try persistent cache first
	if pc := diagcache.GetGlobalCache(); pc != nil {
		if cached, ok := pc.Get(path, content, 24*time.Hour); ok {
			return cached
		}
	}

	// Fall back to in-memory cache
	key := cacheKey(path, content)
	if cached, ok := cacheGet(key); ok {
		return cached
	}
	result := analyze(path, lang, content)
	cachePut(key, result)

	// Also store in persistent cache
	if pc := diagcache.GetGlobalCache(); pc != nil {
		pc.Put(path, content, string(lang), result, 24*time.Hour)
	}

	return result
}

func analyze(path string, lang parsers.Language, content string) []Diagnostic {
	// JSON is handled by a pure-Go parser.
	if lang == parsers.LangJSON {
		return parsers.ParseJSON([]byte(content))
	}

	pr, err := parsers.Parse(context.Background(), lang, []byte(content))
	if err != nil {
		d := Diagnostic{
			Line:     0,
			Severity: "error",
			Code:     "parse-failure",
			Message:  fmt.Sprintf("failed to parse file: %s", err),
			Source:   "orteds",
		}
		d.WithDefaults()
		return []Diagnostic{d}
	}

	var diags []Diagnostic

	// Universal: find ERROR and MISSING nodes in the syntax tree.
	source := fmt.Sprintf("tree-sitter-%s", lang)
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.IsError() {
			text := node.Content(pr.Content)
			if len(text) > 60 {
				text = text[:60] + "…"
			}
			msg := "syntax error"
			if text != "" {
				msg = fmt.Sprintf("syntax error: %s", text)
			}
			d := Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "error",
				Code:       "syntax-error",
				Message:    msg,
				Source:     source,
				Tags:       []string{"syntax"},
				Suggestions: []string{"Check brackets, quotes, and delimiters near the reported location"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		if node.IsMissing() {
			d := Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "error",
				Code:       "missing-token",
				Message:    fmt.Sprintf("missing %s", node.Type()),
				Source:     source,
				Tags:       []string{"syntax"},
				Suggestions: []string{"Add the missing " + node.Type()},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		return true
	})

	// Language-specific rules.
	diags = append(diags, languageRules(path, lang, pr)...)

	// Apply defaults to all language-specific diagnostics
	for i := range diags {
		diags[i].WithDefaults()
	}

	return dedup(diags)
}

// Recover analyzes diagnostics and returns an actionable recovery report.
func Recover(path, content string) RecoveryReport {
	return recovery.Summarize(Analyze(path, content))
}

// RecoverDiagnostics converts diagnostics into actionable recovery guidance.
func RecoverDiagnostics(diags []Diagnostic) RecoveryReport {
	return recovery.Summarize(diags)
}

// RecoverCompilerOutput analyzes compiler output and returns actionable recovery guidance.
func RecoverCompilerOutput(output string, lang compiler.Language) RecoveryReport {
	return recovery.SummarizeCompiler(compiler.ParseOutput(output, lang))
}

// RecoveryMessage returns a user-facing message for the diagnostics.
func RecoveryMessage(path, content string) string {
	return Recover(path, content).Render()
}

// languageRules runs language-specific semantic checks on top of syntax-tree
// walking. It receives the file path because a few rules depend on the exact
// extension (e.g. missing-main only applies to C sources, not headers).
func languageRules(path string, lang parsers.Language, pr *parsers.ParseResult) []Diagnostic {
	switch lang {
	case parsers.LangGo:
		return checkGoRules(pr)
	case parsers.LangPython:
		return checkPythonRules(pr)
	case parsers.LangJavaScript, parsers.LangJSX:
		return checkJavaScriptRules(pr)
	case parsers.LangTypeScript, parsers.LangTSX:
		return checkTypeScriptRules(pr)
	case parsers.LangRust:
		return checkRustRules(pr)
	case parsers.LangJava:
		return checkJavaRules(pr)
	case parsers.LangC:
		return checkCRules(path, pr)
	case parsers.LangCPP:
		return checkCPPRules(pr)
	case parsers.LangCSharp:
		return checkCSharpRules(pr)
	case parsers.LangRuby:
		return checkRubyRules(pr)
	case parsers.LangPHP:
		return checkPHPRules(pr)
	case parsers.LangSwift:
		return checkSwiftRules(pr)
	case parsers.LangKotlin:
		return checkKotlinRules(pr)
	case parsers.LangYAML:
		return checkYAMLRules(pr)
	case parsers.LangTOML:
		return checkTOMLRules(pr)
	}
	return nil
}

// ─── Go rules ────────────────────────────────────────────────────────────────

func checkGoRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	root := pr.Root

	if root.ChildCount() == 0 {
		return diags
	}
	first := root.Child(0)
	if first == nil || first.Type() != "package_clause" {
		d := Diagnostic{
			Line:       1,
			Column:     0,
			EndLine:    1,
			EndColumn:  1,
			Severity:   "error",
			Code:       "missing-package",
			Message:    "Go files must start with a 'package' declaration",
			Source:     "tree-sitter-go",
			Tags:       []string{"syntax", "go"},
			Suggestions: []string{"Add `package <name>` at the top of the file", "Keep the package name consistent with the directory name"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}

	// Add semantic rules
	diags = append(diags, parsers.CheckGoSemanticRules(pr)...)

	return diags
}

// ─── Python rules ─────────────────────────────────────────────────────────────

func checkPythonRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	lines := strings.Split(string(pr.Content), "\n")

	// Detect mixed tabs and spaces in indentation.
	hasTabs, hasSpaces := false, false
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		if line[0] == '\t' {
			hasTabs = true
		} else if line[0] == ' ' {
			hasSpaces = true
		}
		if hasTabs && hasSpaces {
			d := Diagnostic{
				Line:       i + 1,
				Column:     0,
				EndLine:    i + 1,
				EndColumn:  len(line),
				Severity:   "error",
				Code:       "indentation-error",
				Message:    "inconsistent use of tabs and spaces in indentation",
				Source:     "tree-sitter-python",
				Tags:       []string{"style", "python"},
				Suggestions: []string{"Convert all indentation to spaces (recommended: 4 spaces)", "Convert all indentation to tabs", "Configure editor to use consistent indentation"},
			}
			d.WithDefaults()
			diags = append(diags, d)
			break
		}
	}

	// Add semantic rules
	diags = append(diags, parsers.CheckPythonSemanticRules(pr)...)

	return diags
}

// ─── JavaScript rules ────────────────────────────────────────────────────────

func checkJavaScriptRules(pr *parsers.ParseResult) []Diagnostic {
	diags := checkJSCommon(pr, "javascript")
	diags = append(diags, parsers.CheckTSSemanticRules(pr)...)
	return diags
}

func checkTypeScriptRules(pr *parsers.ParseResult) []Diagnostic {
	diags := checkJSCommon(pr, "typescript")
	diags = append(diags, parsers.CheckTSSemanticRules(pr)...)
	return diags
}

func checkJSCommon(pr *parsers.ParseResult, srcLang string) []Diagnostic {
	var diags []Diagnostic
	source := "tree-sitter-" + srcLang

	// Detect 'await' used outside an async function.
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "await_expression" {
			if !insideAsyncFunction(node) {
				d := Diagnostic{
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					EndLine:    int(node.EndPoint().Row) + 1,
					EndColumn:  int(node.EndPoint().Column),
					Severity:   "error",
					Code:       "await-outside-async",
					Message:    "'await' used outside of an async function",
					Source:     source,
					Tags:       []string{"syntax", "async", srcLang},
					Suggestions: []string{"Mark the enclosing function as async", "Remove the await if the function cannot be async", "Wrap in an IIFE if at top level"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})
	return diags
}

// insideAsyncFunction returns true when node is nested inside an async
// function declaration or expression.
func insideAsyncFunction(node *sitter.Node) bool {
	cur := node.Parent()
	for cur != nil {
		t := cur.Type()
		if t == "function_declaration" || t == "function_expression" ||
			t == "arrow_function" || t == "method_definition" {
			// Check for the async modifier – tree-sitter-javascript stores it
			// as a child with type "async".
			count := cur.ChildCount()
			for i := uint32(0); i < count; i++ {
				ch := cur.Child(int(i))
				if ch != nil && ch.Type() == "async" {
					return true
				}
			}
		}
		cur = cur.Parent()
	}
	return false
}

// ─── Rust rules ──────────────────────────────────────────────────────────────

func checkRustRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Rust: detect unclosed macro calls (macro_invocation without a token_tree).
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "macro_invocation" {
			hasBody := false
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "token_tree" {
					hasBody = true
					break
				}
			}
			if !hasBody {
				d := Diagnostic{
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					EndLine:    int(node.EndPoint().Row) + 1,
					EndColumn:  int(node.EndPoint().Column),
					Severity:   "error",
					Code:       "unclosed-macro",
					Message:    "unclosed macro invocation",
					Source:     "tree-sitter-rust",
					Tags:       []string{"syntax", "macro", "rust"},
					Suggestions: []string{"Add the missing token tree (e.g., `{}`, `()`, `[]`)" , "Check for missing closing delimiter", "Verify macro syntax matches definition"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})

	// Add semantic rules
	diags = append(diags, parsers.CheckRustSemanticRules(pr)...)

	return diags
}

// ─── Java rules ──────────────────────────────────────────────────────────────

func checkJavaRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Tree-sitter ERROR nodes cover the primary Java issues.
	diags = append(diags, parsers.CheckJavaSemanticRules(pr)...)
	return diags
}

// ─── C rules ─────────────────────────────────────────────────────────────────

func checkCRules(path string, pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Skip headers: only translation units are expected to define main.
	if strings.EqualFold(filepath.Ext(path), ".h") {
		return diags
	}
	// Check for missing main function (warning, not error)
	hasMain := false
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "function_definition" {
			nameNode := node.ChildByFieldName("declarator")
			if nameNode != nil {
				for i := uint32(0); i < nameNode.ChildCount(); i++ {
					ch := nameNode.Child(int(i))
					if ch != nil && ch.Type() == "identifier" && string(pr.Content[ch.StartByte():ch.EndByte()]) == "main" {
						hasMain = true
						return false
					}
				}
			}
		}
		return true
	})
	if !hasMain {
		d := Diagnostic{
			Line:       1,
			Column:     0,
			Severity:   "info",
			Code:       "missing-main",
			Message:    "no main function found (expected for executables)",
			Source:     "tree-sitter-c",
			Tags:       []string{"style", "c"},
			Suggestions: []string{"Add int main(void) { return 0; } for executables", "Ignore if this is a library/header file"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── C++ rules ───────────────────────────────────────────────────────────────

func checkCPPRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for using namespace std in header files (common issue)
	content := string(pr.Content)
	if strings.Contains(content, "using namespace std") {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, "using namespace std") {
				d := Diagnostic{
					Line:       i + 1,
					Column:     strings.Index(line, "using namespace std"),
					EndLine:    i + 1,
					EndColumn:  strings.Index(line, "using namespace std") + len("using namespace std"),
					Severity:   "warning",
					Code:       "using-namespace-std",
					Message:    "avoid 'using namespace std' in headers (pollutes global namespace)",
					Source:     "tree-sitter-cpp",
					Tags:       []string{"style", "cpp", "best-practice"},
					Suggestions: []string{"Use fully qualified names (std::vector)", "Move using declaration to source file", "Use namespace aliases if needed"},
				}
				d.WithDefaults()
				diags = append(diags, d)
				break
			}
		}
	}
	return diags
}

// ─── C# rules ────────────────────────────────────────────────────────────────

func checkCSharpRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for missing namespace declaration
	hasNamespace := false
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "namespace_declaration" {
			hasNamespace = true
			return false
		}
		return true
	})
	if !hasNamespace {
		d := Diagnostic{
			Line:       1,
			Column:     0,
			Severity:   "warning",
			Code:       "missing-namespace",
			Message:    "file has no namespace declaration",
			Source:     "tree-sitter-csharp",
			Tags:       []string{"style", "csharp", "organization"},
			Suggestions: []string{"Add namespace declaration matching project structure", "Use file-scoped namespace: namespace MyProject;"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── Ruby rules ──────────────────────────────────────────────────────────────

func checkRubyRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for missing frozen_string_literal comment (performance)
	content := string(pr.Content)
	if !strings.HasPrefix(strings.TrimSpace(content), "# frozen_string_literal: true") {
		d := Diagnostic{
			Line:       1,
			Column:     0,
			Severity:   "hint",
			Code:       "missing-frozen-string-literal",
			Message:    "missing frozen_string_literal comment (improves performance)",
			Source:     "tree-sitter-ruby",
			Tags:       []string{"performance", "ruby", "style"},
			Suggestions: []string{"Add # frozen_string_literal: true at top of file", "Configure RuboCop to enforce this"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── PHP rules ───────────────────────────────────────────────────────────────

func checkPHPRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for missing strict_types declaration
	content := string(pr.Content)
	if !strings.Contains(content, "declare(strict_types=1)") {
		d := Diagnostic{
			Line:       1,
			Column:     0,
			Severity:   "hint",
			Code:       "missing-strict-types",
			Message:    "missing strict_types declaration (enables strict type checking)",
			Source:     "tree-sitter-php",
			Tags:       []string{"type-safety", "php", "best-practice"},
			Suggestions: []string{"Add declare(strict_types=1); at top of file", "Enables strict scalar type declarations"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── Swift rules ─────────────────────────────────────────────────────────────

func checkSwiftRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for force unwrapping (!) which can crash
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "force_unwrap_expression" || node.Type() == "forced_unwrap" {
			d := Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "warning",
				Code:       "force-unwrap",
				Message:    "force unwrap (!) can cause runtime crash if nil",
				Source:     "tree-sitter-swift",
				Tags:       []string{"safety", "swift", "crash-risk"},
				Suggestions: []string{"Use optional binding (if let / guard let)", "Use nil-coalescing operator (??)", "Use optional chaining (?.)"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		return true
	})
	return diags
}

// ─── Kotlin rules ────────────────────────────────────────────────────────────

func checkKotlinRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	// Check for !! operator (not-null assertion, can throw NPE)
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "not_null_assertion" {
			d := Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "warning",
				Code:       "not-null-assertion",
				Message:    "!! operator throws NPE if value is null",
				Source:     "tree-sitter-kotlin",
				Tags:       []string{"safety", "kotlin", "null-safety"},
				Suggestions: []string{"Use safe call (?.) with let", "Use elvis operator (?:)", "Add explicit null check"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		return true
	})
	return diags
}

// ─── YAML rules ──────────────────────────────────────────────────────────────

// checkYAMLRules reports duplicate keys within the same mapping. Keys are
// scoped by nesting: a key repeated at a shallower or deeper indentation is
// a different mapping, not a duplicate.
func checkYAMLRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	content := string(pr.Content)
	lines := strings.Split(content, "\n")
	// Stack of open mappings, innermost last; each maps key → first line.
	type scope struct {
		indent int
		keys   map[string]int
	}
	var stack []scope
	push := func(indent int) scope {
		s := scope{indent: indent, keys: make(map[string]int)}
		stack = append(stack, s)
		return s
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		idx := strings.Index(trimmed, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		if strings.HasPrefix(key, "\"") || strings.HasPrefix(key, "'") {
			key = strings.Trim(key, "\"'")
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		// Pop scopes strictly deeper than this line; same-indent siblings
		// belong to the current scope and must not reset it.
		for len(stack) > 0 && stack[len(stack)-1].indent > indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 || stack[len(stack)-1].indent < indent {
			push(indent)
		}
		top := &stack[len(stack)-1]
		if prev, ok := top.keys[key]; ok {
			d := Diagnostic{
				Line:       i + 1,
				Column:     indent,
				EndLine:    i + 1,
				EndColumn:  indent + len(key),
				Severity:   "warning",
				Code:       "duplicate-key",
				Message:    fmt.Sprintf("duplicate key '%s' in the same mapping (first at line %d)", key, prev+1),
				Source:     "tree-sitter-yaml",
				Tags:       []string{"data-integrity", "yaml"},
				Suggestions: []string{"Remove duplicate key", "Merge values if intentional"},
			}
			d.WithDefaults()
			diags = append(diags, d)
			continue
		}
		top.keys[key] = i
	}
	return diags
}

// ─── TOML rules ──────────────────────────────────────────────────────────────

// checkTOMLRules reports duplicate keys within the same table. A new [table]
// header opens a fresh scope, so identical keys under different tables are
// not duplicates.
func checkTOMLRules(pr *parsers.ParseResult) []Diagnostic {
	var diags []Diagnostic
	content := string(pr.Content)
	lines := strings.Split(content, "\n")
	seen := make(map[string]int) // key → first line, within current table
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			seen = make(map[string]int)
			continue
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		if strings.HasPrefix(key, "\"") || strings.HasPrefix(key, "'") {
			key = strings.Trim(key, "\"'")
		}
		if prev, ok := seen[key]; ok {
			d := Diagnostic{
				Line:       i + 1,
				Column:     0,
				EndLine:    i + 1,
				EndColumn:  len(trimmed),
				Severity:   "warning",
				Code:       "duplicate-key",
				Message:    fmt.Sprintf("duplicate key '%s' in the same table (first at line %d)", key, prev+1),
				Source:     "tree-sitter-toml",
				Tags:       []string{"data-integrity", "toml"},
				Suggestions: []string{"Remove duplicate key", "Use arrays/tables for multiple values"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		} else {
			seen[key] = i
		}
	}
	return diags
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// dedup removes duplicate diagnostics (same key).
func dedup(diags []Diagnostic) []Diagnostic {
	seen := make(map[string]struct{}, len(diags))
	out := make([]Diagnostic, 0, len(diags))
	for _, d := range diags {
		k := d.Key()
		if _, exists := seen[k]; !exists {
			seen[k] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}
