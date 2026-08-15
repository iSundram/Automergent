package render

import (
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

var defaultRenderer *glamour.TermRenderer
var widthRenderers sync.Map

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

// MarkdownWithWidth renders markdown wrapped to the available terminal width.
func MarkdownWithWidth(content string, width int) string {
	if width <= 0 {
		return Markdown(content)
	}
	value, ok := widthRenderers.Load(width)
	if !ok {
		style := styles.DarkStyleConfig
		zeroMargin := uint(0)
		style.Document.Margin = &zeroMargin
		style.H1.Prefix = ""
		style.H1.Suffix = ""
		style.H1.BackgroundColor = nil
		style.H2.Prefix = ""
		style.H3.Prefix = ""
		style.H4.Prefix = ""
		style.H5.Prefix = ""
		style.H6.Prefix = ""
		style.HorizontalRule.Format = "\n"
		style.Code.Prefix = " "
		style.Code.Suffix = " "
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStyles(style),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return content
		}
		value, _ = widthRenderers.LoadOrStore(width, renderer)
	}
	renderer := value.(*glamour.TermRenderer)
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}
