package render

import (
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
)

// Code syntax-highlights a code block.
func Code(content, language string) string {
	if language == "" {
		language = "text"
	}
	var sb strings.Builder
	if err := quick.Highlight(&sb, content, language, "terminal256", "monokai"); err != nil {
		return content
	}
	return sb.String()
}
