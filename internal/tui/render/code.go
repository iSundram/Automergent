package render

import (
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
)

// Code syntax-highlights a code block using the active theme's chroma style
// (set via SetTheme; defaults to monokai before any theme is applied).
func Code(content, language string) string {
	if language == "" {
		language = "text"
	}
	var sb strings.Builder
	if err := quick.Highlight(&sb, content, language, "terminal256", CurrentSyntaxStyle()); err != nil {
		return content
	}
	return sb.String()
}
