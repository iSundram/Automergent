// Package parsers provides semantic analysis rules for TypeScript/JavaScript.
package parsers

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// CheckTSSemanticRules runs semantic checks for TypeScript/JavaScript code.
func CheckTSSemanticRules(pr *ParseResult) []types.Diagnostic {
	var diags []types.Diagnostic
	content := string(pr.Content)

	// Check for unused variables (let/const declarations not referenced)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "lexical_declaration" || node.Type() == "variable_declaration" {
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "variable_declarator" {
					nameNode := ch.ChildByFieldName("name")
					if nameNode != nil && nameNode.Type() == "identifier" {
						varName := content[nameNode.StartByte():nameNode.EndByte()]
						if isUnusedTSVar(pr, varName, ch) {
							d := types.Diagnostic{
								Line:       int(nameNode.StartPoint().Row) + 1,
								Column:     int(nameNode.StartPoint().Column),
								EndLine:    int(nameNode.EndPoint().Row) + 1,
								EndColumn:  int(nameNode.EndPoint().Column),
								Severity:   "warning",
								Code:       "unused-variable",
								Message:    "'" + varName + "' is declared but its value is never read",
								Source:     "ts-semantic",
								Tags:       []string{"typescript", "unused", "style"},
								Suggestions: []string{"Use the variable", "Prefix with _ to indicate intentionally unused", "Remove if not needed"},
							}
							d.WithDefaults()
							diags = append(diags, d)
						}
					}
				}
			}
		}
		return true
	})

	// Check for explicit 'any' type usage
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "type_annotation" {
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "any_type" {
					d := types.Diagnostic{
						Line:       int(ch.StartPoint().Row) + 1,
						Column:     int(ch.StartPoint().Column),
						EndLine:    int(ch.EndPoint().Row) + 1,
						EndColumn:  int(ch.EndPoint().Column),
						Severity:   "warning",
						Code:       "explicit-any",
						Message:    "avoid using 'any' type (loses type safety)",
						Source:     "ts-semantic",
						Tags:       []string{"typescript", "type-safety", "any"},
						Suggestions: []string{"Use specific type instead of any", "Use 'unknown' if type is truly unknown", "Add proper type annotation"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for non-null assertion (!) usage
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "non_null_assertion" {
			d := types.Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "warning",
				Code:       "non-null-assertion",
				Message:    "non-null assertion (!) can cause runtime errors if value is null/undefined",
				Source:     "ts-semantic",
				Tags:       []string{"typescript", "null-safety", "runtime-risk"},
				Suggestions: []string{"Use optional chaining (?.) instead", "Add explicit null/undefined check", "Use type guard to narrow type"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		return true
	})

	// Check for console.log in production code (warning)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call_expression" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil && funcNode.Type() == "member_expression" {
				obj := funcNode.ChildByFieldName("object")
				prop := funcNode.ChildByFieldName("property")
				if obj != nil && prop != nil {
					objName := content[obj.StartByte():obj.EndByte()]
					propName := content[prop.StartByte():prop.EndByte()]
					if objName == "console" && (propName == "log" || propName == "debug" || propName == "info") {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "hint",
							Code:       "console-log",
							Message:    "console." + propName + " should be removed or replaced with proper logging in production",
							Source:     "ts-semantic",
							Tags:       []string{"typescript", "debug", "production"},
							Suggestions: []string{"Remove console.log before committing", "Use proper logging library (winston, pino, etc.)", "Use debug module for conditional logging"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for missing return type on public functions
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "function_declaration" || node.Type() == "method_definition" {
			// Check if it's exported (has export modifier or is in export statement)
			isExported := false
			parent := node.Parent()
			if parent != nil && parent.Type() == "export_statement" {
				isExported = true
			}
			// Check for export modifier
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "export" {
					isExported = true
					break
				}
			}

			if isExported {
				// Check if return type is missing
				returnType := node.ChildByFieldName("return_type")
				if returnType == nil {
					nameNode := node.ChildByFieldName("name")
					funcName := "anonymous"
					if nameNode != nil {
						funcName = content[nameNode.StartByte():nameNode.EndByte()]
					}
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "hint",
						Code:       "missing-return-type",
						Message:    "exported function '" + funcName + "' should have explicit return type",
						Source:     "ts-semantic",
						Tags:       []string{"typescript", "type-safety", "documentation"},
						Suggestions: []string{"Add explicit return type annotation", "Use 'void' if function returns nothing", "TypeScript can infer but explicit is better for public API"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for == instead of === (loose equality)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "binary_expression" {
			operator := node.ChildByFieldName("operator")
			if operator != nil {
				op := content[operator.StartByte():operator.EndByte()]
				if op == "==" || op == "!=" {
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "warning",
						Code:       "loose-equality",
						Message:    "use strict equality (===/!==) instead of loose equality (" + op + ")",
						Source:     "ts-semantic",
						Tags:       []string{"typescript", "equality", "best-practice"},
						Suggestions: []string{"Use === instead of ==", "Use !== instead of !=", "Strict equality avoids type coercion bugs"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for for...in without hasOwnProperty check
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "for_in_statement" {
			d := types.Diagnostic{
				Line:       int(node.StartPoint().Row) + 1,
				Column:     int(node.StartPoint().Column),
				EndLine:    int(node.EndPoint().Row) + 1,
				EndColumn:  int(node.EndPoint().Column),
				Severity:   "warning",
				Code:       "for-in-without-hasown",
				Message:    "for...in loops should check hasOwnProperty or use Object.keys/for...of",
				Source:     "ts-semantic",
				Tags:       []string{"typescript", "iteration", "prototype-pollution"},
				Suggestions: []string{"Use for...of with Object.keys()", "Add hasOwnProperty check inside loop", "Use Object.entries() for key-value iteration"},
			}
			d.WithDefaults()
			diags = append(diags, d)
		}
		return true
	})

	return diags
}

func isUnusedTSVar(pr *ParseResult, varName string, declNode *sitter.Node) bool {
	content := string(pr.Content)
	declPos := declNode.EndByte()
	remaining := content[declPos:]

	// Count references (exclude declaration)
	usageCount := strings.Count(remaining, varName)
	// Also check for destructuring patterns
	return usageCount == 0
}