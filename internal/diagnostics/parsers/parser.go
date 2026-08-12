// Package parsers provides tree-sitter parser wrappers for supported languages.
package parsers

import (
	"context"
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

// Language represents a detected programming language identifier.
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

// ParseResult holds the parsed syntax tree together with its source content.
type ParseResult struct {
	Root     *sitter.Node
	Content  []byte
	Language Language
}

// languageRegistry maps language identifiers to their tree-sitter Language objects.
var languageRegistry = map[Language]*sitter.Language{
	LangGo:         golang.GetLanguage(),
	LangPython:     python.GetLanguage(),
	LangJavaScript: javascript.GetLanguage(),
	LangJSX:        javascript.GetLanguage(), // JSX is handled by the JS grammar
	LangTypeScript: typescript.GetLanguage(),
	LangTSX:        tsx.GetLanguage(),
	LangRust:       rust.GetLanguage(),
	LangJava:       java.GetLanguage(),
	LangC:          c.GetLanguage(),
	LangCPP:        cpp.GetLanguage(),
	LangCSharp:     csharp.GetLanguage(),
	LangRuby:       ruby.GetLanguage(),
	LangPHP:        php.GetLanguage(),
	LangSwift:      swift.GetLanguage(),
	LangKotlin:     kotlin.GetLanguage(),
	LangLua:        lua.GetLanguage(),
	LangYAML:       yaml.GetLanguage(),
	LangTOML:       toml.GetLanguage(),
	// JSON is handled by a pure-Go parser (see json_parser.go).
}

// GetSitterLanguage returns the tree-sitter Language for lang, or nil when
// the language uses a non-tree-sitter parser.
func GetSitterLanguage(lang Language) *sitter.Language {
	return languageRegistry[lang]
}

// Parse parses content for the given language and returns a ParseResult.
// For languages without a tree-sitter grammar the function returns an error.
func Parse(ctx context.Context, lang Language, content []byte) (*ParseResult, error) {
	sitterLang := GetSitterLanguage(lang)
	if sitterLang == nil {
		return nil, fmt.Errorf("no tree-sitter grammar for language %q", lang)
	}

	p := sitter.NewParser()
	p.SetLanguage(sitterLang)

	tree, err := p.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("parse error for %s: %w", lang, err)
	}

	return &ParseResult{
		Root:     tree.RootNode(),
		Content:  content,
		Language: lang,
	}, nil
}

// Walk visits every node in the tree in depth-first pre-order, calling fn for
// each node. Walk continues until all nodes have been visited or fn returns
// false.
func Walk(node *sitter.Node, fn func(*sitter.Node) bool) {
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}
	count := node.ChildCount()
	for i := uint32(0); i < count; i++ {
		Walk(node.Child(int(i)), fn)
	}
}
