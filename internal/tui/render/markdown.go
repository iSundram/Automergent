package render

import (
	"strings"

	"charm.land/glamour/v2"
)

var defaultRenderer *glamour.TermRenderer

func init() {
	// Use a reasonable default width - glamour handles wrapping internally
	// and lipgloss will constrain the final output
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(0), // Disable word wrap - let lipgloss handle it
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
	return strings.TrimSpace(rendered)
}
