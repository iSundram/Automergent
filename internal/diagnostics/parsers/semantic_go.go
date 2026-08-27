// Package parsers provides semantic analysis rules for Go.
package parsers

import (
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

	// Check for error handling (functions returning error but not checked)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil {
				funcName := content[funcNode.StartByte():funcNode.EndByte()]
				if returnsError(funcName, pr) && !errorIsChecked(node, pr) {
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "warning",
						Code:       "unchecked-error",
						Message:    "function '" + funcName + "' returns error but it's not checked",
						Source:     "go-semantic",
						Tags:       []string{"go", "error-handling", "best-practice"},
						Suggestions: []string{"Check the returned error", "Use '_' to explicitly ignore if intentional", "Handle error appropriately"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	return diags
}

func isUnusedVar(pr *ParseResult, varName string, declNode *sitter.Node) bool {
	// Simple heuristic: search for references after declaration
	content := string(pr.Content)
	declPos := declNode.EndByte()

	// Look for usage after declaration
	remaining := content[declPos:]
	// Skip the declaration itself
	usageCount := strings.Count(remaining, varName)
	return usageCount == 0
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
				if containsVarDecl(ch, varName) {
					return true
				}
			}
		}
		parent = parent.Parent()
	}
	return false
}

func containsVarDecl(node *sitter.Node, varName string) bool {
	found := false
	Walk(node, func(n *sitter.Node) bool {
		if (n.Type() == "short_var_declaration" || n.Type() == "var_declaration") {
			for i := uint32(0); i < n.ChildCount(); i++ {
				ch := n.Child(int(i))
				if ch != nil && ch.Type() == "identifier" {
					if string(ch.Content([]byte(ch.Content(nil)))) == varName {
						found = true
						return false
					}
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

func returnsError(funcName string, pr *ParseResult) bool {
	// Known functions that return error
	errorFuncs := []string{"Open", "Read", "Write", "Close", "Create", "Remove", "Mkdir",
		"Unmarshal", "Marshal", "NewDecoder", "NewEncoder", "Do", "Get", "Post",
		"Query", "Exec", "Scan", "QueryRow", "Begin", "Commit", "Rollback"}
	for _, f := range errorFuncs {
		if strings.Contains(funcName, f) {
			return true
		}
	}
	return false
}

func errorIsChecked(callNode *sitter.Node, pr *ParseResult) bool {
	// Check if parent is an assignment to error variable or if statement
	parent := callNode.Parent()
	if parent != nil {
		if parent.Type() == "assignment_statement" || parent.Type() == "short_var_declaration" {
			// Check if one of the LHS variables is named err or error
			return true
		}
		if parent.Type() == "if_statement" {
			// Error checked in if condition
			return true
		}
	}
	return false
}