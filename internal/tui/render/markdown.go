package render

import (
	"strconv"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

var (
	defaultRenderer *glamour.TermRenderer
	widthRenderers  sync.Map // "dark|width" -> *glamour.TermRenderer
)

func init() {
	defaultRenderer = newGlamourRenderer(tameStyle(baseStyleConfig(true)), 0)
}

// tameStyle removes the loud document chrome from glamour's default styles:
// no margins, no heading prefixes/backgrounds, thin code fences.
func tameStyle(base ansi.StyleConfig) ansi.StyleConfig {
	style := base
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
	return style
}

// Markdown renders markdown text to terminal-formatted output.
func Markdown(content string) string {
	mu.RLock()
	r := defaultRenderer
	dark := darkMarkdown
	mu.RUnlock()
	if r == nil {
		r = newGlamourRenderer(tameStyle(baseStyleConfig(dark)), 0)
		if r == nil {
			return content
		}
	}
	rendered, err := r.Render(content)
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
	mu.RLock()
	dark := darkMarkdown
	key := strconv.FormatBool(dark) + "|" + strconv.Itoa(width)
	mu.RUnlock()

	value, ok := widthRenderers.Load(key)
	if !ok {
		renderer := newGlamourRenderer(tameStyle(baseStyleConfig(dark)), width)
		if renderer == nil {
			return content
		}
		value, _ = widthRenderers.LoadOrStore(key, renderer)
	}
	renderer := value.(*glamour.TermRenderer)
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}

func baseStyleConfig(dark bool) ansi.StyleConfig {
	if dark {
		return styles.DarkStyleConfig
	}
	return styles.LightStyleConfig
}

func newGlamourRenderer(base ansi.StyleConfig, width int) *glamour.TermRenderer {
	opts := []glamour.TermRendererOption{
		glamour.WithStyles(base),
		glamour.WithWordWrap(width),
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return nil
	}
	return r
}
