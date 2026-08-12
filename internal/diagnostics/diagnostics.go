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
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/iSundram/Automergent/internal/diagnostics/compiler"
	"github.com/iSundram/Automergent/internal/diagnostics/parsers"
	"github.com/iSundram/Automergent/internal/diagnostics/recovery"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// Diagnostic is re-exported from types for convenience.
type Diagnostic = types.Diagnostic

// RecoveryReport summarizes diagnostics into actionable recovery guidance.
type RecoveryReport = recovery.Report

// Analyze parses content as the language inferred from path and returns all
// detected diagnostics.  It returns nil when the language is unsupported or
// when the file exceeds the configured size limit.
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

func analyzeWithCache(path string, lang parsers.Language, content string) []Diagnostic {
	key := cacheKey(path, content)
	if cached, ok := cacheGet(key); ok {
		return cached
	}
	result := analyze(lang, content)
	cachePut(key, result)
	return result
}

func analyze(lang parsers.Language, content string) []Diagnostic {
	// JSON is handled by a pure-Go parser.
	if lang == parsers.LangJSON {
		return parsers.ParseJSON([]byte(content))
	}

	pr, err := parsers.Parse(context.Background(), lang, []byte(content))
	if err != nil {
		return []Diagnostic{{
			Line:     0,
			Severity: "error",
			Code:     "parse-failure",
			Message:  fmt.Sprintf("failed to parse file: %s", err),
			Source:   "orteds",
		}}
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
			diags = append(diags, Diagnostic{
				Line:     int(node.StartPoint().Row) + 1,
				Column:   int(node.StartPoint().Column),
				Severity: "error",
				Code:     "syntax-error",
				Message:  msg,
				Source:   source,
			})
		}
		if node.IsMissing() {
			diags = append(diags, Diagnostic{
				Line:     int(node.StartPoint().Row) + 1,
				Column:   int(node.StartPoint().Column),
				Severity: "error",
				Code:     "missing-token",
				Message:  fmt.Sprintf("missing %s", node.Type()),
				Source:   source,
			})
		}
		return true
	})

	// Language-specific rules.
	diags = append(diags, languageRules(lang, pr)...)

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
// walking.
func languageRules(lang parsers.Language, pr *parsers.ParseResult) []Diagnostic {
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
		return checkCRules(pr)
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
	case parsers.LangLua:
		return checkLuaRules(pr)
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
		diags = append(diags, Diagnostic{
			Line:     1,
			Severity: "error",
			Code:     "missing-package",
			Message:  "Go files must start with a 'package' declaration",
			Source:   "tree-sitter-go",
		})
	}
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
			diags = append(diags, Diagnostic{
				Line:     i + 1,
				Severity: "error",
				Code:     "indentation-error",
				Message:  "inconsistent use of tabs and spaces in indentation",
				Source:   "tree-sitter-python",
			})
			break
		}
	}
	return diags
}

// ─── JavaScript rules ────────────────────────────────────────────────────────

func checkJavaScriptRules(pr *parsers.ParseResult) []Diagnostic {
	return checkJSCommon(pr, "javascript")
}

func checkTypeScriptRules(pr *parsers.ParseResult) []Diagnostic {
	return checkJSCommon(pr, "typescript")
}

func checkJSCommon(pr *parsers.ParseResult, srcLang string) []Diagnostic {
	var diags []Diagnostic
	source := "tree-sitter-" + srcLang

	// Detect 'await' used outside an async function.
	parsers.Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "await_expression" {
			if !insideAsyncFunction(node) {
				diags = append(diags, Diagnostic{
					Line:     int(node.StartPoint().Row) + 1,
					Column:   int(node.StartPoint().Column),
					Severity: "error",
					Code:     "await-outside-async",
					Message:  "'await' used outside of an async function",
					Source:   source,
				})
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
				diags = append(diags, Diagnostic{
					Line:     int(node.StartPoint().Row) + 1,
					Column:   int(node.StartPoint().Column),
					Severity: "error",
					Code:     "unclosed-macro",
					Message:  "unclosed macro invocation",
					Source:   "tree-sitter-rust",
				})
			}
		}
		return true
	})
	return diags
}

// ─── Java rules ──────────────────────────────────────────────────────────────

func checkJavaRules(pr *parsers.ParseResult) []Diagnostic {
	// Tree-sitter ERROR nodes cover the primary Java issues.
	return nil
}

// ─── C rules ─────────────────────────────────────────────────────────────────

func checkCRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── C++ rules ───────────────────────────────────────────────────────────────

func checkCPPRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── C# rules ────────────────────────────────────────────────────────────────

func checkCSharpRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── Ruby rules ──────────────────────────────────────────────────────────────

func checkRubyRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── PHP rules ───────────────────────────────────────────────────────────────

func checkPHPRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── Swift rules ─────────────────────────────────────────────────────────────

func checkSwiftRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── Kotlin rules ────────────────────────────────────────────────────────────

func checkKotlinRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── Lua rules ───────────────────────────────────────────────────────────────

func checkLuaRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── YAML rules ──────────────────────────────────────────────────────────────

func checkYAMLRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── TOML rules ──────────────────────────────────────────────────────────────

func checkTOMLRules(pr *parsers.ParseResult) []Diagnostic {
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// dedup removes duplicate diagnostics (same key).
func dedup(diags []Diagnostic) []Diagnostic {
	seen := make(map[string]struct{}, len(diags))
	out := diags[:0]
	for _, d := range diags {
		k := d.Key()
		if _, exists := seen[k]; !exists {
			seen[k] = struct{}{}
			out = append(out, d)
		}
	}
	return out
}
