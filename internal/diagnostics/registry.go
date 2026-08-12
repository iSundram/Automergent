// Package registry provides file-extension to language detection.
package diagnostics

import (
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/diagnostics/parsers"
)

// extensionMap maps lowercase file extensions (with dot) to language IDs.
var extensionMap = map[string]parsers.Language{
	// Tier 1
	".go":   parsers.LangGo,
	".py":   parsers.LangPython,
	".js":   parsers.LangJavaScript,
	".jsx":  parsers.LangJSX,
	".ts":   parsers.LangTypeScript,
	".tsx":  parsers.LangTSX,
	".rs":   parsers.LangRust,
	".java": parsers.LangJava,
	// Tier 2
	".c":   parsers.LangC,
	".h":   parsers.LangC,
	".cpp": parsers.LangCPP,
	".hpp": parsers.LangCPP,
	".cc":  parsers.LangCPP,
	".cs":  parsers.LangCSharp,
	".rb":  parsers.LangRuby,
	".php": parsers.LangPHP,
	// Tier 3
	".swift": parsers.LangSwift,
	".kt":    parsers.LangKotlin,
	".kts":   parsers.LangKotlin,
	".lua":   parsers.LangLua,
	// Config files
	".json": parsers.LangJSON,
	".yaml": parsers.LangYAML,
	".yml":  parsers.LangYAML,
	".toml": parsers.LangTOML,
}

// DetectLanguage returns the language for the given file path based on its
// extension. Returns an empty Language when the extension is not recognised.
func DetectLanguage(path string) parsers.Language {
	ext := strings.ToLower(filepath.Ext(path))
	return extensionMap[ext]
}
