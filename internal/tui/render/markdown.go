package render

// Markdown rendering.
//
// Every color here is derived from the active themes.Theme rather than from
// glamour's built-in dark/light configs. Those configs carry their own fixed
// palette (and their own embedded chroma table for fenced code), which meant
// assistant prose ignored the user's theme entirely and fenced code was
// highlighted by a different engine than tool output.

import (
	"image/color"
	"strconv"
	"strings"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	// xansi is the terminal-string toolkit; `ansi` above is glamour's style
	// package, and both are needed here.
	xansi "github.com/charmbracelet/x/ansi"
)

// maxWidthRenderers bounds the renderer cache. A drag-resize walks through one
// width per frame, and each glamour renderer is not free to retain.
const maxWidthRenderers = 16

var (
	defaultRenderer *glamour.TermRenderer

	rendererMu     sync.Mutex
	widthRenderers = map[string]*glamour.TermRenderer{}
)

func init() {
	// mdPalette is a package-level var, so it is already built from the default
	// theme by the time init runs. SetTheme rebuilds this renderer.
	defaultRenderer = newGlamourRenderer(themedStyleConfig(mdPalette, true, syntaxStyle), 0)
}

// dropRenderers clears the renderer cache. Callers must hold mu; this function
// takes no locks of its own so SetTheme can call it while holding the write
// lock.
func dropRenderers() {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	widthRenderers = map[string]*glamour.TermRenderer{}
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }
func uintPtr(u uint) *uint    { return &u }

// themedStyleConfig builds a glamour style from the markdown palette.
//
// The shape is deliberately tame — no margins, no heading backgrounds, no
// document indent — because assistant output is already inside a labelled
// bubble. What the palette drives is color and weight, and it is the same
// palette inline.go uses, so the live line and the finalized block agree.
func themedStyleConfig(p MarkdownPalette, dark bool, chromaStyle string) ansi.StyleConfig {
	style := baseStyleConfig(dark)

	var (
		text    = hexOf(p.Text)
		subtext = hexOf(p.Subtext)
		muted   = hexOf(p.Muted)
		code    = hexOf(p.Code)
		codeBg  = hexOf(p.CodeBg)
		bullet  = hexOf(p.Bullet)
		quote   = hexOf(p.Quote)
		rule    = hexOf(p.Rule)
		link    = hexOf(p.LinkText)
		linkURL = hexOf(p.LinkURL)
	)

	zero := uintPtr(0)

	// Document: no chrome at all. The bubble supplies the frame.
	style.Document.Margin = zero
	style.Document.Indent = zero
	style.Document.StylePrimitive.Color = strPtr(text)
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""

	// Text carries NO color of its own. glamour renders a text node by cascading
	// Styles.Text over the enclosing block's style, and a child color always
	// wins — so setting one here silently repainted every heading, every
	// blockquote and every table cell in the body color.
	style.Text = ansi.StylePrimitive{}
	style.Paragraph.Margin = zero
	style.Paragraph.Indent = zero

	// Headings: descending emphasis down the palette rather than descending
	// size, which a terminal cannot express. No prefixes or backgrounds — the
	// weight and hue carry the level.
	heading := func(c color.Color) ansi.StyleBlock {
		return ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:       strPtr(hexOf(c)),
				Bold:        boolPtr(true),
				BlockPrefix: "",
				BlockSuffix: "",
				Prefix:      "",
				Suffix:      "",
			},
			Margin: zero,
			Indent: zero,
		}
	}
	style.Heading = heading(p.Heading[0])
	style.H1 = heading(p.Heading[1])
	style.H1.BackgroundColor = nil
	style.H2 = heading(p.Heading[2])
	style.H3 = heading(p.Heading[3])
	style.H4 = heading(p.Heading[4])
	style.H5 = heading(p.Heading[5])
	style.H6 = heading(p.Heading[6])

	// Inline emphasis.
	style.Strong = ansi.StylePrimitive{Color: strPtr(text), Bold: boolPtr(true)}
	style.Emph = ansi.StylePrimitive{Color: strPtr(subtext), Italic: boolPtr(true)}
	style.Strikethrough = ansi.StylePrimitive{Color: strPtr(muted), CrossedOut: boolPtr(true)}

	// Links: the text is what you read, the URL is supporting detail.
	style.Link = ansi.StylePrimitive{Color: strPtr(linkURL), Underline: boolPtr(false)}
	style.LinkText = ansi.StylePrimitive{Color: strPtr(link), Underline: boolPtr(true)}
	style.Image = ansi.StylePrimitive{Color: strPtr(linkURL)}
	style.ImageText = ansi.StylePrimitive{Color: strPtr(link), Format: "{{.text}}"}

	// Lists: a themed bullet, one indent level, no extra margin.
	style.List.Margin = zero
	style.List.Indent = uintPtr(1)
	style.List.LevelIndent = 2
	style.List.StylePrimitive.Color = strPtr(text)
	style.Item = ansi.StylePrimitive{Color: strPtr(bullet), Prefix: "• "}
	style.Enumeration = ansi.StylePrimitive{Color: strPtr(bullet), Prefix: ". "}
	style.Task = ansi.StyleTask{
		StylePrimitive: ansi.StylePrimitive{Color: strPtr(text)},
		Ticked:         TaskToken(p, true),
		Unticked:       TaskToken(p, false),
	}

	// Block quote: a themed rule down the left edge instead of glamour's
	// indent-only treatment, so a quote is distinguishable from a code block at
	// a glance. IndentToken (not Prefix) is what glamour repeats per line.
	//
	// The token is one cell wide on purpose. glamour's IndentWriter emits it once
	// per indent unit while reserving exactly one *column* per unit, so the
	// conventional two-cell "│ " makes every wrapped quote line one cell wider
	// than the width it was wrapped to — and the terminal then wraps it again.
	// The token is pre-colored here because glamour styles it with the *parent*
	// block's primitive (Document), which would paint the bar the same color as
	// the prose.
	style.BlockQuote = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:  strPtr(quote),
			Italic: boolPtr(true),
		},
		Margin:      zero,
		Indent:      uintPtr(1),
		IndentToken: strPtr(QuoteToken(p.QuoteBar)),
	}

	// Inline code: a blue-tinted fill reads as "verbatim" without competing with
	// the green the diff renderer and most chroma styles already claim.
	style.Code = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:           strPtr(code),
			BackgroundColor: strPtr(codeBg),
			Prefix:          " ",
			Suffix:          " ",
		},
	}

	// Fenced code: hand the block to chroma under the SAME style name that
	// render.Code uses, so a Go snippet looks identical whether it arrived in
	// prose or in a tool result. Chroma must be nil for Theme to take effect.
	style.CodeBlock = ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Color: strPtr(text)},
			Margin:         zero,
			Indent:         uintPtr(1),
		},
		Theme:  chromaStyle,
		Chroma: nil,
	}

	// Tables and rules.
	style.Table = ansi.StyleTable{
		StyleBlock:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr(text)}, Margin: zero},
		CenterSeparator: strPtr("┼"),
		ColumnSeparator: strPtr("│"),
		RowSeparator:    strPtr("─"),
	}
	style.HorizontalRule = ansi.StylePrimitive{
		Color:  strPtr(rule),
		Format: "\n" + RuleFormat + "\n",
	}

	style.DefinitionTerm = ansi.StylePrimitive{Color: strPtr(hexOf(p.Heading[0])), Bold: boolPtr(true)}
	style.DefinitionDescription = ansi.StylePrimitive{Color: strPtr(subtext)}
	style.HTMLBlock = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr(muted)}}
	style.HTMLSpan = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr(muted)}}

	return style
}

// Markdown renders markdown text to terminal-formatted output, unwrapped.
//
// Prefer MarkdownWithWidth: unwrapped output placed inside a fixed-width box
// gets re-wrapped by lipgloss, which does not respect the ANSI spans glamour
// emitted and can split them.
func Markdown(content string) string {
	mu.RLock()
	r := defaultRenderer
	mu.RUnlock()
	if r == nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return trimTrailingBlank(rendered)
}

// MarkdownWithWidth renders markdown wrapped to the available terminal width.
func MarkdownWithWidth(content string, width int) string {
	if width <= 0 {
		return Markdown(content)
	}
	mu.RLock()
	p := mdPalette
	dark := darkMarkdown
	chroma := syntaxStyle
	gen := markdownGen
	mu.RUnlock()

	key := strconv.FormatUint(gen, 10) + "|" + strconv.Itoa(width)

	rendererMu.Lock()
	renderer, ok := widthRenderers[key]
	if !ok {
		renderer = newGlamourRenderer(themedStyleConfig(p, dark, chroma), width)
		if renderer != nil {
			if len(widthRenderers) >= maxWidthRenderers {
				widthRenderers = map[string]*glamour.TermRenderer{}
			}
			widthRenderers[key] = renderer
		}
	}
	rendererMu.Unlock()

	if renderer == nil {
		return content
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}
	return trimTrailingBlank(rendered)
}

func baseStyleConfig(dark bool) ansi.StyleConfig {
	if dark {
		return styles.DarkStyleConfig
	}
	return styles.LightStyleConfig
}

// trimTrailingBlank strips the blank lines glamour pads a document with, while
// leaving indentation on the first content line intact. A plain
// strings.TrimSpace would eat the leading indent of a code block or nested list
// that happens to open the message.
//
// Blankness is tested after stripping ANSI: glamour pads short lines by writing
// styled spaces, so a "blank" separator line is really a run of SGR sequences
// and TrimSpace alone reports it as content. That is why extra empty rows used
// to appear above lists and quotes.
func trimTrailingBlank(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	blank := func(line string) bool {
		return strings.TrimSpace(xansi.Strip(line)) == ""
	}
	start := 0
	for start < len(lines) && blank(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && blank(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
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
