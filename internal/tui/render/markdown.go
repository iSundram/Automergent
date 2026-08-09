package render

import (
	"github.com/charmbracelet/glamour"
)

var defaultRenderer *glamour.TermRenderer

func init() {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err == nil {
		defaultRenderer = r
	}
}

// Markdown renders markdown text to terminal-formatted output.
func Markdown(content string) string {
	if defaultRenderer == nil {
		return content
	}
	rendered, err := defaultRenderer.Render(content)
	if err != nil {
		return content
	}
	return rendered
}
