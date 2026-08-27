// Package parsers provides semantic analysis rules for Python.
package parsers

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// CheckPythonSemanticRules runs semantic checks for Python code.
func CheckPythonSemanticRules(pr *ParseResult) []types.Diagnostic {
	var diags []types.Diagnostic
	content := string(pr.Content)

	// Check for unused imports
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "import_statement" || node.Type() == "import_from_statement" {
			// Get imported names
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && (ch.Type() == "dotted_name" || ch.Type() == "identifier" || ch.Type() == "aliased_import") {
					importName := content[ch.StartByte():ch.EndByte()]
					// Handle aliased imports
					if ch.Type() == "aliased_import" {
						// Get the alias name
						for j := uint32(0); j < ch.ChildCount(); j++ {
							aliasCh := ch.Child(int(j))
							if aliasCh != nil && aliasCh.Type() == "identifier" {
								importName = content[aliasCh.StartByte():aliasCh.EndByte()]
								break
							}
						}
					}
					if isUnusedImport(pr, importName, node, content) {
						d := types.Diagnostic{
							Line:       int(ch.StartPoint().Row) + 1,
							Column:     int(ch.StartPoint().Column),
							EndLine:    int(ch.EndPoint().Row) + 1,
							EndColumn:  int(ch.EndPoint().Column),
							Severity:   "warning",
							Code:       "unused-import",
							Message:    "imported '" + importName + "' but not used",
							Source:     "python-semantic",
							Tags:       []string{"python", "unused", "import"},
							Suggestions: []string{"Remove unused import", "Use the imported name", "Add # noqa comment if intentional"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for undefined variables (referenced before assignment)
	// Temporarily disabled due to scope analysis complexity
	// Walk(pr.Root, func(node *sitter.Node) bool {
	// 	if node.Type() == "identifier" {
	// 		parent := node.Parent()
	// 		if parent != nil && isVariableReference(node, parent) {
	// 			varName := content[node.StartByte():node.EndByte()]
	// 			if isUndefinedVar(pr, varName, node, content) {
	// 				d := types.Diagnostic{...}
	// 				d.WithDefaults()
	// 				diags = append(diags, d)
	// 			}
	// 		}
	// 	}
	// 	return true
	// })

	// Check for mutable default arguments
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "default_parameter" {
			valueNode := node.ChildByFieldName("value")
			if valueNode != nil && isMutableLiteral(valueNode) {
				d := types.Diagnostic{
					Line:       int(valueNode.StartPoint().Row) + 1,
					Column:     int(valueNode.StartPoint().Column),
					EndLine:    int(valueNode.EndPoint().Row) + 1,
					EndColumn:  int(valueNode.EndPoint().Column),
					Severity:   "warning",
					Code:       "mutable-default-arg",
					Message:    "mutable default argument (list/dict/set) can cause unexpected behavior",
					Source:     "python-semantic",
					Tags:       []string{"python", "mutable-default", "bug-risk"},
					Suggestions: []string{"Use None as default and create new object inside function", "Use tuple/frozenset for immutable defaults", "Document the mutable default behavior if intentional"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})

	// Check for bare except: clauses
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "except_clause" {
			// Check if it has no exception type (bare except)
			hasType := false
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && ch.Type() == "exception_type" {
					hasType = true
					break
				}
			}
			if !hasType {
				d := types.Diagnostic{
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					EndLine:    int(node.EndPoint().Row) + 1,
					EndColumn:  int(node.EndPoint().Column),
					Severity:   "warning",
					Code:       "bare-except",
					Message:    "bare 'except:' catches all exceptions including SystemExit and KeyboardInterrupt",
					Source:     "python-semantic",
					Tags:       []string{"python", "exception-handling", "best-practice"},
					Suggestions: []string{"Use 'except Exception:' to catch only expected errors", "Specify exact exception types to catch", "Avoid bare except in production code"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})

	// Check for comparison with True/False/None using == instead of is
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "comparison_operator" {
			// Check for == True, == False, == None
			for i := uint32(0); i < node.ChildCount(); i++ {
				ch := node.Child(int(i))
				if ch != nil && (ch.Type() == "true" || ch.Type() == "false" || ch.Type() == "none") {
					d := types.Diagnostic{
						Line:       int(ch.StartPoint().Row) + 1,
						Column:     int(ch.StartPoint().Column),
						EndLine:    int(ch.EndPoint().Row) + 1,
						EndColumn:  int(ch.EndPoint().Column),
						Severity:   "hint",
						Code:       "compare-with-const",
						Message:    "use 'is' instead of '==' for comparison with True/False/None",
						Source:     "python-semantic",
						Tags:       []string{"python", "style", "identity"},
						Suggestions: []string{"Use 'is True' instead of '== True'", "Use 'is False' instead of '== False'", "Use 'is None' instead of '== None'"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for missing docstrings on public functions/classes
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "function_definition" || node.Type() == "class_definition" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				name := content[nameNode.StartByte():nameNode.EndByte()]
				// Check if public (not starting with _)
				if !strings.HasPrefix(name, "_") {
					// Check if first statement is a docstring
					bodyNode := node.ChildByFieldName("body")
					if bodyNode != nil && bodyNode.ChildCount() > 1 {
						firstStmt := bodyNode.Child(1) // Child 0 is usually ':'
						if firstStmt == nil || firstStmt.Type() != "expression_statement" {
							d := types.Diagnostic{
								Line:       int(node.StartPoint().Row) + 1,
								Column:     int(node.StartPoint().Column),
								EndLine:    int(node.EndPoint().Row) + 1,
								EndColumn:  int(node.EndPoint().Column),
								Severity:   "hint",
								Code:       "missing-docstring",
								Message:    "public " + node.Type() + " '" + name + "' missing docstring",
								Source:     "python-semantic",
								Tags:       []string{"python", "documentation", "style"},
								Suggestions: []string{"Add docstring describing purpose, args, and returns", "Follow Google/NumPy/Sphinx docstring format", "Use type hints in addition to docstring"},
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

	// Check for print() statements (should use logging in production)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "call" {
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil && funcNode.Type() == "identifier" {
				funcName := content[funcNode.StartByte():funcNode.EndByte()]
				if funcName == "print" {
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "hint",
						Code:       "print-statement",
						Message:    "print() should be replaced with logging in production code",
						Source:     "python-semantic",
						Tags:       []string{"python", "debug", "production"},
						Suggestions: []string{"Use logging module instead", "Use logging.info/debug/warning/error", "Configure logging for production"},
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

func isVariableReference(node, parent *sitter.Node) bool {
	// Identifier is a reference if parent is not a definition context
	defContexts := map[string]bool{
		"function_definition": true, "class_definition": true,
		"parameter": true, "typed_parameter": true, "default_parameter": true,
		"assignment": true, "with_clause": true,
		"import_statement": true, "import_from_statement": true,
	}
	if defContexts[parent.Type()] {
		return false
	}
	// Also check grandparent for parameter context
	grandparent := parent.Parent()
	if grandparent != nil && grandparent.Type() == "parameters" {
		return false
	}
	// Check if parent is a parameter-like node (handle variations)
	if strings.Contains(parent.Type(), "parameter") {
		return false
	}
	return true
}

func isUnusedImport(pr *ParseResult, importName string, importNode *sitter.Node, content string) bool {
	importPos := importNode.EndByte()
	remaining := content[importPos:]

	// Extract simple name (last part after .)
	parts := strings.Split(importName, ".")
	simpleName := parts[len(parts)-1]

	// Count references after import
	usageCount := strings.Count(remaining, simpleName)
	return usageCount == 0
}

func isUndefinedVar(pr *ParseResult, varName string, refNode *sitter.Node, content string) bool {
	// Check if variable is defined before this reference in the same scope
	refStartByte := refNode.StartByte()

	// Walk up to find scope (function, class, module)
	// Keep going until we find a node with parameters field or hit module
	scope := refNode
	foundFunction := false
	for scope != nil {
		if scope.Type() == "module" {
			break
		}
		if scope.ChildByFieldName("parameters") != nil {
			foundFunction = true
			break // Found function definition
		}
		scope = scope.Parent()
	}
	if scope == nil || !foundFunction {
		// If we can't reliably find the function scope, don't flag
		return false
	}

	// Check if it's a function parameter (at definition site)
	parent := refNode.Parent()
	if parent != nil && strings.Contains(parent.Type(), "parameter") {
		return false // Parameters are defined
	}
	// Check if it's a function call (function name)
	if parent != nil && parent.Type() == "call" {
		funcNode := parent.ChildByFieldName("function")
		if funcNode == refNode {
			return false // Function being called is not a variable reference
		}
	}

	// Check function parameters in the scope directly (before walking)
	paramsNode := scope.ChildByFieldName("parameters")
	if paramsNode != nil {
		for i := uint32(0); i < paramsNode.ChildCount(); i++ {
			ch := paramsNode.Child(int(i))
			if ch != nil && strings.Contains(ch.Type(), "parameter") {
				for j := uint32(0); j < ch.ChildCount(); j++ {
					paramIdent := ch.Child(int(j))
					if paramIdent != nil && paramIdent.Type() == "identifier" {
						if content[paramIdent.StartByte():paramIdent.EndByte()] == varName {
							return false // Found in function parameters
						}
					}
				}
			}
		}
	}

	// Search for assignment before reference line
	found := false
	Walk(scope, func(n *sitter.Node) bool {
		// Only check nodes that start before the reference
		if n.StartByte() >= refStartByte {
			return true
		}
		if n.Type() == "assignment" || n.Type() == "annotated_assignment" {
			left := n.ChildByFieldName("left")
			if left != nil {
				for i := uint32(0); i < left.ChildCount(); i++ {
					ch := left.Child(int(i))
					if ch != nil && ch.Type() == "identifier" {
						if content[ch.StartByte():ch.EndByte()] == varName {
							found = true
							return false
						}
					}
				}
			}
		}
		return true
	})
	return !found
}

func isMutableLiteral(node *sitter.Node) bool {
	mutableTypes := map[string]bool{
		"list": true, "list_comprehension": true,
		"dictionary": true, "dict_comprehension": true,
		"set": true, "set_comprehension": true,
	}
	return mutableTypes[node.Type()]
}