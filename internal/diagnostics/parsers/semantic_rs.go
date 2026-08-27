// Package parsers provides semantic analysis rules for Rust.
package parsers

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// CheckRustSemanticRules runs semantic checks for Rust code.
func CheckRustSemanticRules(pr *ParseResult) []types.Diagnostic {
	var diags []types.Diagnostic
	content := string(pr.Content)

	// Check for unused variables (let bindings not used)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "let_declaration" {
			// Get pattern (variable name)
			patternNode := node.ChildByFieldName("pattern")
			if patternNode != nil && patternNode.Type() == "identifier" {
				varName := content[patternNode.StartByte():patternNode.EndByte()]
				// Skip if starts with _ (intentionally unused)
				if strings.HasPrefix(varName, "_") {
					return true
				}
				if isUnusedRustVar(pr, varName, node) {
					d := types.Diagnostic{
						Line:       int(patternNode.StartPoint().Row) + 1,
						Column:     int(patternNode.StartPoint().Column),
						EndLine:    int(patternNode.EndPoint().Row) + 1,
						EndColumn:  int(patternNode.EndPoint().Column),
						Severity:   "warning",
						Code:       "unused-variable",
						Message:    "unused variable: `" + varName + "`",
						Source:     "rust-semantic",
						Tags:       []string{"rust", "unused", "style"},
						Suggestions: []string{"Use the variable", "Prefix with _ to indicate intentionally unused (e.g., _var)", "Remove if not needed"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for unwrap() usage on Option/Result (can panic)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil && funcNode.Type() == "field_expression" {
				fieldNode := funcNode.ChildByFieldName("field")
				if fieldNode != nil {
					methodName := content[fieldNode.StartByte():fieldNode.EndByte()]
					if methodName == "unwrap" || methodName == "expect" {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "warning",
							Code:       "unwrap-used",
							Message:    "use of ." + methodName + "() can panic if value is None/Err",
							Source:     "rust-semantic",
							Tags:       []string{"rust", "panic-risk", "error-handling"},
							Suggestions: []string{"Use match or if let to handle Option/Result", "Use ? operator to propagate error", "Use unwrap_or/unwrap_or_else for defaults", "Use expect with descriptive message if panic is acceptable"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for clone() usage (potential performance issue)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil && funcNode.Type() == "field_expression" {
				fieldNode := funcNode.ChildByFieldName("field")
				if fieldNode != nil {
					methodName := content[fieldNode.StartByte():fieldNode.EndByte()]
					if methodName == "clone" {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "hint",
							Code:       "clone-used",
							Message:    ".clone() creates a deep copy (potential performance cost)",
							Source:     "rust-semantic",
							Tags:       []string{"rust", "performance", "allocation"},
							Suggestions: []string{"Consider using references (&) instead of owned values", "Use Arc/Rc for shared ownership", "Profile to confirm clone is bottleneck"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for TODO/FIXME comments
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "line_comment" || node.Type() == "block_comment" {
			commentText := strings.ToUpper(content[node.StartByte():node.EndByte()])
			if strings.Contains(commentText, "TODO") || strings.Contains(commentText, "FIXME") || strings.Contains(commentText, "XXX") || strings.Contains(commentText, "HACK") {
				d := types.Diagnostic{
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					EndLine:    int(node.EndPoint().Row) + 1,
					EndColumn:  int(node.EndPoint().Column),
					Severity:   "hint",
					Code:       "todo-comment",
					Message:    "TODO/FIXME comment found",
					Source:     "rust-semantic",
					Tags:       []string{"rust", "todo", "technical-debt"},
					Suggestions: []string{"Address the TODO/FIXME", "Create issue/ticket for tracking", "Remove if resolved"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})

	// Check for missing Debug derive on public structs
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "struct_item" {
			// Check if public
			isPublic := false
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "visibility_modifier" {
					visText := content[ch.StartByte():ch.EndByte()]
					if strings.Contains(visText, "pub") {
						isPublic = true
						break
					}
				}
			}
			if isPublic {
				// Check for Debug derive
				hasDebug := false
				for i := uint32(0); i < node.ChildCount(); i++ {
					ch := node.Child(int(i))
					if ch != nil && ch.Type() == "attribute" {
						attrText := content[ch.StartByte():ch.EndByte()]
						if strings.Contains(attrText, "Debug") || strings.Contains(attrText, "derive") {
							hasDebug = true
							break
						}
					}
				}
				if !hasDebug {
					nameNode := node.ChildByFieldName("name")
					structName := "unknown"
					if nameNode != nil {
						structName = content[nameNode.StartByte():nameNode.EndByte()]
					}
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "hint",
						Code:       "missing-debug-derive",
						Message:    "public struct `" + structName + "` should derive Debug for debugging",
						Source:     "rust-semantic",
						Tags:       []string{"rust", "derive", "debugging"},
						Suggestions: []string{"Add #[derive(Debug)] to struct", "Add #[derive(Clone, Debug)] if cloning needed", "Use #[derive(PartialEq, Debug)] for comparisons"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for panic! usage
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "macro_invocation" {
			// Check if it's panic!
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "macro_name" {
					macroName := content[ch.StartByte():ch.EndByte()]
					if macroName == "panic" || macroName == "unreachable" {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "warning",
							Code:       "panic-macro",
							Message:    macroName + "!() causes program termination",
							Source:     "rust-semantic",
							Tags:       []string{"rust", "panic", "termination"},
							Suggestions: []string{"Return Result/Option instead of panicking", "Use expect with context if panic is intentional", "Consider using Result type for error handling"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	return diags
}

func isUnusedRustVar(pr *ParseResult, varName string, declNode *sitter.Node) bool {
	content := string(pr.Content)
	declPos := declNode.EndByte()
	remaining := content[declPos:]

	// Count references after declaration
	usageCount := strings.Count(remaining, varName)
	return usageCount == 0
}