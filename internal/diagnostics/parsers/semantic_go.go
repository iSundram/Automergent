// Package parsers provides semantic analysis rules for Go.
package parsers

import (
	"regexp"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// CheckGoSemanticRules runs semantic checks for Go code beyond syntax errors.
func CheckGoSemanticRules(pr *ParseResult) []types.Diagnostic {
	var diags []types.Diagnostic
	content := string(pr.Content)

	// Check for unused variables (basic heuristic: declared but not referenced)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "short_var_declaration" || node.Type() == "var_declaration" {
			// Check if variables are used
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "identifier" {
					varName := content[ch.StartByte():ch.EndByte()]
					if isUnusedVar(pr, varName, ch) {
						d := types.Diagnostic{
							Line:       int(ch.StartPoint().Row) + 1,
							Column:     int(ch.StartPoint().Column),
							EndLine:    int(ch.EndPoint().Row) + 1,
							EndColumn:  int(ch.EndPoint().Column),
							Severity:   "warning",
							Code:       "unused-variable",
							Message:    "variable '" + varName + "' declared but not used",
							Source:     "go-semantic",
							Tags:       []string{"go", "unused", "style"},
							Suggestions: []string{"Use the variable", "Prefix with _ to intentionally discard", "Remove if not needed"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for shadowed variables
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "short_var_declaration" {
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "identifier" {
					varName := content[ch.StartByte():ch.EndByte()]
					if isShadowed(pr, varName, ch) {
						d := types.Diagnostic{
							Line:       int(ch.StartPoint().Row) + 1,
							Column:     int(ch.StartPoint().Column),
							EndLine:    int(ch.EndPoint().Row) + 1,
							EndColumn:  int(ch.EndPoint().Column),
							Severity:   "warning",
							Code:       "shadowed-variable",
							Message:    "variable '" + varName + "' shadows variable from outer scope",
							Source:     "go-semantic",
							Tags:       []string{"go", "shadow", "style"},
							Suggestions: []string{"Rename variable to avoid shadowing", "Use different variable name"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for unreachable code after return/panic
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "return_statement" || node.Type() == "panic_call" {
			// Check next sibling for statements
			parent := node.Parent()
			if parent != nil {
				found := false
				for i := uint32(0); i < parent.ChildCount(); i++ {
					ch := parent.Child(int(i))
					if ch == node {
						found = true
						continue
					}
					if found && ch != nil && isStatement(ch) {
						d := types.Diagnostic{
							Line:       int(ch.StartPoint().Row) + 1,
							Column:     int(ch.StartPoint().Column),
							EndLine:    int(ch.EndPoint().Row) + 1,
							EndColumn:  int(ch.EndPoint().Column),
							Severity:   "warning",
							Code:       "unreachable-code",
							Message:    "unreachable code after return/panic",
							Source:     "go-semantic",
							Tags:       []string{"go", "unreachable", "logic"},
							Suggestions: []string{"Remove unreachable code", "Check control flow logic"},
						}
						d.WithDefaults()
						diags = append(diags, d)
						break
					}
				}
			}
		}
		return true
	})

	// Check for printf format mismatches
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil {
				funcName := content[funcNode.StartByte():funcNode.EndByte()]
				if isPrintfLike(funcName) {
					if hasFormatMismatch(pr, node, content) {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "warning",
							Code:       "printf-format-mismatch",
							Message:    "printf format string may not match arguments",
							Source:     "go-semantic",
							Tags:       []string{"go", "printf", "type-safety"},
							Suggestions: []string{"Verify format specifiers match argument types", "Use fmt.Printf with correct verbs"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Note: unchecked-error detection is deliberately absent — a text
	// heuristic (call name contains "Get"/"Read"/…) misfires on idiomatic
	// code like `defer f.Close()`. Real coverage belongs to errcheck via
	// the linters package.

	return diags
}

func isUnusedVar(pr *ParseResult, varName string, declNode *sitter.Node) bool {
	// Single- and double-letter names produce too many substring hits to be
	// trustworthy with a text heuristic.
	if len(varName) <= 2 {
		return false
	}
	// Skip blank and explicitly-discarded identifiers.
	if varName == "_" || strings.HasPrefix(varName, "_") {
		return false
	}
	content := string(pr.Content)
	declPos := declNode.EndByte()

	// Count whole-word occurrences after the declaration; a bare substring
	// count reports var "n" as used because "nil" contains "n".
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(varName) + `\b`)
	return !re.MatchString(content[declPos:])
}

func isShadowed(pr *ParseResult, varName string, declNode *sitter.Node) bool {
	// Check parent scopes for same variable name
	parent := declNode.Parent()
	for parent != nil {
		if parent.Type() == "block" || parent.Type() == "function_body" {
			// Check siblings before this declaration
			for i := uint32(0); i < parent.ChildCount(); i++ {
				ch := parent.Child(int(i))
				if ch == declNode {
					break
				}
				if containsVarDecl(pr, ch, varName) {
					return true
				}
			}
		}
		parent = parent.Parent()
	}
	return false
}

func containsVarDecl(pr *ParseResult, node *sitter.Node, varName string) bool {
	if node == nil {
		return false
	}
	found := false
	Walk(node, func(n *sitter.Node) bool {
		if n.Type() == "short_var_declaration" || n.Type() == "var_declaration" {
			for i := uint32(0); i < n.ChildCount(); i++ {
				ch := n.Child(int(i))
				if ch != nil && ch.Type() == "identifier" &&
					string(pr.Content[ch.StartByte():ch.EndByte()]) == varName {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func isStatement(node *sitter.Node) bool {
	stmtTypes := map[string]bool{
		"expression_statement": true, "assignment_statement": true,
		"short_var_declaration": true, "var_declaration": true,
		"if_statement": true, "for_statement": true,
		"return_statement": true, "call_expression": true,
	}
	return stmtTypes[node.Type()]
}

func isPrintfLike(name string) bool {
	printfFuncs := []string{"Printf", "Sprintf", "Fprintf", "Errorf", "Fatalf", "Logf", "Printf"}
	for _, f := range printfFuncs {
		if strings.HasSuffix(name, f) {
			return true
		}
	}
	return false
}

func hasFormatMismatch(pr *ParseResult, callNode *sitter.Node, content string) bool {
	// Simplified: just check if there are format verbs but wrong arg count
	// Full implementation would parse format string and compare with args
	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return false
	}
	argCount := 0
	for i := uint32(0); i < argsNode.ChildCount(); i++ {
		ch := argsNode.Child(int(i))
		if ch != nil && ch.Type() != "," && ch.Type() != "(" && ch.Type() != ")" {
			argCount++
		}
	}
// Find format string (first string argument)
			for i := uint32(0); i < argsNode.ChildCount(); i++ {
				ch := argsNode.Child(int(i))
				if ch != nil && (ch.Type() == "interpreted_string_literal" || ch.Type() == "raw_string_literal") {
					fmtStr := content[ch.StartByte():ch.EndByte()]
					verbCount := strings.Count(fmtStr, "%") - strings.Count(fmtStr, "%%")
					return verbCount != argCount-1 // -1 for format string itself
				}
			}
			return false
}