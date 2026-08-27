// Package parsers provides semantic analysis rules for Java.
package parsers

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// CheckJavaSemanticRules runs semantic checks for Java code.
func CheckJavaSemanticRules(pr *ParseResult) []types.Diagnostic {
	var diags []types.Diagnostic
	content := string(pr.Content)

	// Check for unused imports
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "import_declaration" {
			// Get imported class name
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				importName := content[nameNode.StartByte():nameNode.EndByte()]
				if isUnusedJavaImport(pr, importName, node) {
					d := types.Diagnostic{
						Line:       int(nameNode.StartPoint().Row) + 1,
						Column:     int(nameNode.StartPoint().Column),
						EndLine:    int(nameNode.EndPoint().Row) + 1,
						EndColumn:  int(nameNode.EndPoint().Column),
						Severity:   "warning",
						Code:       "unused-import",
						Message:    "unused import: " + importName,
						Source:     "java-semantic",
						Tags:       []string{"java", "unused", "import"},
						Suggestions: []string{"Remove unused import", "Use the imported class", "Organize imports with IDE"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for raw types (generics not used)
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "type_identifier" {
			// Check if it's a generic type used without type parameters
			parent := node.Parent()
			if parent != nil && (parent.Type() == "variable_declarator" || parent.Type() == "field_declaration" || parent.Type() == "formal_parameter") {
				typeName := content[node.StartByte():node.EndByte()]
				if isRawType(typeName) && !hasTypeArguments(node) {
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "warning",
						Code:       "raw-type",
						Message:    "raw type '" + typeName + "' should be parameterized",
						Source:     "java-semantic",
						Tags:       []string{"java", "generics", "type-safety"},
						Suggestions: []string{"Add type parameters (e.g., List<String>)", "Use diamond operator (<>) for inference", "Specify explicit type arguments"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for System.out.println in production code
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "method_invocation" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil && nameNode.Type() == "identifier" {
				methodName := content[nameNode.StartByte():nameNode.EndByte()]
				if methodName == "println" || methodName == "print" {
					// Check if it's System.out
					objNode := node.ChildByFieldName("object")
					if objNode != nil && objNode.Type() == "field_access" {
						objText := content[objNode.StartByte():objNode.EndByte()]
						if strings.Contains(objText, "System.out") {
							d := types.Diagnostic{
								Line:       int(node.StartPoint().Row) + 1,
								Column:     int(node.StartPoint().Column),
								EndLine:    int(node.EndPoint().Row) + 1,
								EndColumn:  int(node.EndPoint().Column),
								Severity:   "hint",
								Code:       "system-out-print",
								Message:    "System.out.println/print should use proper logging framework in production",
								Source:     "java-semantic",
								Tags:       []string{"java", "logging", "production"},
								Suggestions: []string{"Use SLF4J/Log4j/Logback logger", "Use logger.info/debug/warn/error", "Configure logging for production environment"},
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

	// Check for empty catch blocks
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "catch_clause" {
			bodyNode := node.ChildByFieldName("body")
			if bodyNode != nil && bodyNode.Type() == "block" {
				// Check if block is empty (only braces)
				if bodyNode.ChildCount() <= 2 { // Just { }
					d := types.Diagnostic{
						Line:       int(node.StartPoint().Row) + 1,
						Column:     int(node.StartPoint().Column),
						EndLine:    int(node.EndPoint().Row) + 1,
						EndColumn:  int(node.EndPoint().Column),
						Severity:   "warning",
						Code:       "empty-catch-block",
						Message:    "empty catch block swallows exceptions silently",
						Source:     "java-semantic",
						Tags:       []string{"java", "exception-handling", "bug-risk"},
						Suggestions: []string{"Handle the exception appropriately", "Log the exception at minimum", "Re-throw if cannot handle", "Add comment explaining why exception is ignored"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for == instead of .equals() for String comparison
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "binary_expression" {
			operatorNode := node.ChildByFieldName("operator")
			if operatorNode != nil {
				op := content[operatorNode.StartByte():operatorNode.EndByte()]
				if op == "==" || op == "!=" {
					// Check if operands are String type
					left := node.ChildByFieldName("left")
					right := node.ChildByFieldName("right")
					if isStringType(left, pr) || isStringType(right, pr) {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "warning",
							Code:       "string-equality",
							Message:    "use .equals() instead of " + op + " for String comparison",
							Source:     "java-semantic",
							Tags:       []string{"java", "string", "equality", "bug-risk"},
							Suggestions: []string{"Use str1.equals(str2) for content comparison", "Use Objects.equals(str1, str2) for null-safe comparison", "Use str1 == str2 only for reference comparison"},
						}
						d.WithDefaults()
						diags = append(diags, d)
					}
				}
			}
		}
		return true
	})

	// Check for missing @Override annotation
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "method_declaration" {
			// Check if it overrides a parent method
			modifiersNode := node.ChildByFieldName("modifiers")
			hasOverride := false
			if modifiersNode != nil {
				for i := uint32(0); i < modifiersNode.ChildCount(); i++ {
					ch := modifiersNode.Child(int(i))
					if ch != nil && ch.Type() == "marker_annotation" {
						annoText := content[ch.StartByte():ch.EndByte()]
						if strings.Contains(annoText, "Override") {
							hasOverride = true
							break
						}
					}
				}
			}

			// Check if method name matches common override patterns
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				methodName := content[nameNode.StartByte():nameNode.EndByte()]
				commonOverrides := map[string]bool{
					"toString": true, "equals": true, "hashCode": true,
					"compareTo": true, "run": true, "clone": true,
					"finalize": true, "close": true,
				}
				if commonOverrides[methodName] && !hasOverride {
					d := types.Diagnostic{
						Line:       int(nameNode.StartPoint().Row) + 1,
						Column:     int(nameNode.StartPoint().Column),
						EndLine:    int(nameNode.EndPoint().Row) + 1,
						EndColumn:  int(nameNode.EndPoint().Column),
						Severity:   "hint",
						Code:       "missing-override",
						Message:    "method '" + methodName + "' likely overrides parent method but missing @Override annotation",
						Source:     "java-semantic",
						Tags:       []string{"java", "override", "annotation", "best-practice"},
						Suggestions: []string{"Add @Override annotation", "Verify method signature matches parent", "Remove if not intended to override"},
					}
					d.WithDefaults()
					diags = append(diags, d)
				}
			}
		}
		return true
	})

	// Check for TODO/FIXME comments
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "line_comment" || node.Type() == "block_comment" {
			commentText := strings.ToUpper(content[node.StartByte():node.EndByte()])
			if strings.Contains(commentText, "TODO") || strings.Contains(commentText, "FIXME") || strings.Contains(commentText, "XXX") {
				d := types.Diagnostic{
					Line:       int(node.StartPoint().Row) + 1,
					Column:     int(node.StartPoint().Column),
					EndLine:    int(node.EndPoint().Row) + 1,
					EndColumn:  int(node.EndPoint().Column),
					Severity:   "hint",
					Code:       "todo-comment",
					Message:    "TODO/FIXME comment found",
					Source:     "java-semantic",
					Tags:       []string{"java", "todo", "technical-debt"},
					Suggestions: []string{"Address the TODO/FIXME", "Create issue/ticket for tracking", "Remove if resolved"},
				}
				d.WithDefaults()
				diags = append(diags, d)
			}
		}
		return true
	})

	// Check for nullable annotations missing
	Walk(pr.Root, func(node *sitter.Node) bool {
		if node.Type() == "method_declaration" || node.Type() == "field_declaration" {
			// Check for return type or field type that could be null
			typeNode := node.ChildByFieldName("type")
			if typeNode != nil {
				typeText := content[typeNode.StartByte():typeNode.EndByte()]
				// Skip primitives
				primitives := map[string]bool{
					"int": true, "long": true, "double": true, "float": true,
					"boolean": true, "char": true, "byte": true, "short": true, "void": true,
				}
				if !primitives[typeText] {
					// Check for @Nullable or @NonNull annotations
					modifiersNode := node.ChildByFieldName("modifiers")
					hasNullable := false
					if modifiersNode != nil {
						for i := uint32(0); i < modifiersNode.ChildCount(); i++ {
							ch := modifiersNode.Child(int(i))
							if ch != nil && ch.Type() == "marker_annotation" {
								annoText := content[ch.StartByte():ch.EndByte()]
								if strings.Contains(annoText, "Nullable") || strings.Contains(annoText, "NonNull") {
									hasNullable = true
									break
								}
							}
						}
					}
					if !hasNullable {
						d := types.Diagnostic{
							Line:       int(node.StartPoint().Row) + 1,
							Column:     int(node.StartPoint().Column),
							EndLine:    int(node.EndPoint().Row) + 1,
							EndColumn:  int(node.EndPoint().Column),
							Severity:   "hint",
							Code:       "missing-nullable-annotation",
							Message:    "non-primitive type '" + typeText + "' should have @Nullable or @NonNull annotation",
							Source:     "java-semantic",
							Tags:       []string{"java", "null-safety", "annotation"},
							Suggestions: []string{"Add @NonNull if value cannot be null", "Add @Nullable if value can be null", "Use Optional<T> for return types"},
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

func isUnusedJavaImport(pr *ParseResult, importName string, importNode *sitter.Node) bool {
	content := string(pr.Content)
	importPos := importNode.EndByte()
	remaining := content[importPos:]

	// Extract simple class name (last part after .)
	parts := strings.Split(importName, ".")
	className := parts[len(parts)-1]

	// Count references after import
	usageCount := strings.Count(remaining, className)
	return usageCount == 0
}

func isRawType(typeName string) bool {
	rawTypes := map[string]bool{
		"List": true, "Set": true, "Map": true, "Collection": true,
		"ArrayList": true, "HashSet": true, "HashMap": true, "LinkedList": true,
		"TreeSet": true, "TreeMap": true, "Queue": true, "Deque": true,
		"Iterator": true, "Iterable": true, "Comparable": true, "Comparator": true,
		"Optional": true, "Supplier": true, "Consumer": true, "Function": true,
		"Predicate": true, "Stream": true, "Future": true, "CompletableFuture": true,
	}
	return rawTypes[typeName]
}

func hasTypeArguments(node *sitter.Node) bool {
	for i := uint32(0); i < node.ChildCount(); i++ {
		ch := node.Child(int(i))
		if ch != nil && ch.Type() == "type_arguments" {
			return true
		}
	}
	return false
}

func isStringType(node *sitter.Node, pr *ParseResult) bool {
	if node == nil {
		return false
	}
	// Check if type is String
	if node.Type() == "type_identifier" {
		typeName := pr.Content[node.StartByte():node.EndByte()]
		return string(typeName) == "String"
	}
	// Check parent for type info
	return false
}