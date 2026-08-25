package render

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// MarkdownPalette is every color the markdown renderers use.
//
// It exists because there are two of them: glamour renders finalized blocks and
// the inline styler renders the live streaming line. If they derive their colors
// independently they drift, and the drift is visible — text changes color the
// instant a newline lands, which reads as the UI re-rendering something it had
// already got right. One struct, one derivation, no drift.
type MarkdownPalette struct {
	Text    color.Color
	Subtext color.Color
	Muted   color.Color

	Heading [7]color.Color // index by level; [0] is the generic heading color

	Code   color.Color // inline code foreground
	CodeBg color.Color // inline code fill

	LinkText color.Color
	LinkURL  color.Color

	Bullet   color.Color
	QuoteBar color.Color
	Quote    color.Color
	Rule     color.Color

	Checked   color.Color
	Unchecked color.Color

	Cursor color.Color
}

// DiffPalette holds every color the diff renderers need, derived from the
// active theme so all output follows the user's palette instead of hardcoded
// hex values.
type DiffPalette struct {
	AddFg     color.Color
	DelFg     color.Color
	AddBg     color.Color
	DelBg     color.Color
	PrefixAdd color.Color // background for the + prefix cell
	PrefixDel color.Color // background for the - prefix cell
	Hunk      color.Color
	File      color.Color
	LineNum   color.Color
	Context   color.Color
	WordDelBg color.Color // background behind struck-through deleted words
	WordAddBg color.Color // background behind inserted words
}

var (
	mu           sync.RWMutex
	activeTheme  *themes.Theme
	syntaxStyle  = "monokai" // chroma style name
	darkMarkdown = true      // glamour dark vs light base style
	diffPalette  = buildDiffPalette(themes.Get("catppuccin"))
	mdPalette    = buildMarkdownPalette(themes.Get("catppuccin"))
	// markdownGen is bumped on every theme switch. Cached renderers and cached
	// rendered output are keyed by it, so a switch between two themes of the
	// same brightness (which a "dark|width" key could not distinguish) still
	// invalidates everything.
	markdownGen uint64
)

// MarkdownColors returns the active markdown palette.
func MarkdownColors() MarkdownPalette {
	mu.RLock()
	defer mu.RUnlock()
	return mdPalette
}

// buildMarkdownPalette derives the markdown colors from a theme.
//
// Code is deliberately in the blue half of the palette rather than green: green
// is what the diff renderer uses for additions and what most chroma styles use
// for string literals, so a green code span read as "something was added" and
// competed with the syntax highlighting inside fenced blocks right next to it.
func buildMarkdownPalette(t *themes.Theme) MarkdownPalette {
	if t == nil {
		t = themes.Get("catppuccin")
	}
	bg := t.Background
	return MarkdownPalette{
		Text:    t.Text,
		Subtext: t.Subtext,
		Muted:   t.Muted,
		// Headings descend the palette rather than descending in size, which a
		// terminal cannot express.
		Heading: [7]color.Color{
			t.Accent,
			t.Accent, t.AccentAlt, t.Blue, t.Cyan, t.Subtext, t.Muted,
		},
		Code: t.Cyan,
		// A blue-tinted fill instead of the flat surface color: it separates a
		// code span from the prose without the heavy block a full surface swatch
		// draws mid-sentence.
		CodeBg:    themes.Mix(bg, t.Blue, 0.20),
		LinkText:  t.Blue,
		LinkURL:   t.Muted,
		Bullet:    t.Accent,
		QuoteBar:  t.Blue,
		Quote:     t.Subtext,
		Rule:      t.BorderNormal,
		Checked:   t.Green,
		Unchecked: t.Muted,
		Cursor:    t.Accent,
	}
}

// MarkdownGeneration returns a counter that changes whenever the markdown
// styling changes. Callers that cache rendered markdown key on it.
func MarkdownGeneration() uint64 {
	mu.RLock()
	defer mu.RUnlock()
	return markdownGen
}

// quoteBarGlyph is the left rule drawn beside a block quote.
//
// It is exactly one cell wide, which is a hard requirement rather than a taste
// call: glamour's IndentWriter emits the indent token once per indent unit while
// reserving one *column* per unit, so a two-cell "│ " overflows every wrapped
// quote line by one cell. A left-eighth block gives the bar visual separation
// from the text without a second column.
const quoteBarGlyph = "▎"

// QuoteToken returns the block-quote rule, pre-styled in the given color.
//
// Both markdown renderers call this: glamour uses it as its IndentToken and the
// inline styler emits it for a live quote line. Sharing the exact string is what
// stops the bar from changing shape or color when a line is finalized.
func QuoteToken(bar color.Color) string {
	if bar == nil {
		return quoteBarGlyph
	}
	return lipgloss.NewStyle().Foreground(bar).Render(quoteBarGlyph)
}

// TaskToken returns the checkbox for a task-list item, pre-styled.
//
// Pre-styling is not an optimization here. glamour renders Task.Ticked and
// Task.Unticked with the *enclosing list's* style primitive rather than with
// Task's own, so putting the color in the string is the only way to give a
// ticked box a different color from an unticked one — and the only way for the
// inline styler to emit a byte-identical glyph.
func TaskToken(p MarkdownPalette, checked bool) string {
	glyph, c := "[ ] ", p.Unchecked
	if checked {
		glyph, c = "[✓] ", p.Checked
	}
	if c == nil {
		return glyph
	}
	return lipgloss.NewStyle().Foreground(c).Render(glyph)
}

// RuleFormat is the thematic-break glyph run. glamour wraps it in newlines via
// HorizontalRule.Format; the inline styler emits it bare.
const RuleFormat = "────────"

// ActiveTheme returns the theme currently driving every renderer. It is never
// nil: before the first SetTheme it reports the built-in default.
func ActiveTheme() *themes.Theme {
	mu.RLock()
	defer mu.RUnlock()
	if activeTheme == nil {
		return themes.Get("catppuccin")
	}
	return activeTheme
}

// hexOf formats a color as "#rrggbb" for glamour, whose StyleConfig takes
// colors as strings rather than color.Color. Unlike colorHex it tolerates a nil
// color, because a theme may legitimately leave a slot unset.
func hexOf(c color.Color) string {
	if c == nil {
		return ""
	}
	return colorHex(c)
}

// SyntaxStyleFor maps a theme name to the closest chroma style. Unknown names
// fall back based on the theme's own brightness.
func SyntaxStyleFor(themeName string, dark bool) string {
	switch strings.ToLower(themeName) {
	case "catppuccin":
		return "catppuccin-mocha"
	case "dracula":
		return "dracula"
	case "nord":
		return "nord"
	case "gruvbox":
		if dark {
			return "gruvbox"
		}
		return "gruvbox-light"
	case "onedark":
		return "onedark"
	case "tokyonight":
		return "tokyonight-night"
	case "solarized-dark":
		return "solarized-dark256"
	case "solarized-light":
		return "solarized-light"
	case "monokai", "modern":
		return "monokai"
	case "high-contrast":
		return "hr_high_contrast"
	default:
		if dark {
			return "github-dark"
		}
		return "github"
	}
}

// SetTheme re-derives every renderer's colors from a theme. Call after any
// theme switch (startup or /theme).
func SetTheme(t *themes.Theme) {
	if t == nil {
		return
	}
	dark := true
	if t.Background != nil {
		dark = themes.IsDark(t.Background)
	}
	mu.Lock()
	defer mu.Unlock()
	activeTheme = t
	darkMarkdown = dark
	syntaxStyle = SyntaxStyleFor(t.Name, dark)
	diffPalette = buildDiffPalette(t)
	mdPalette = buildMarkdownPalette(t)
	markdownGen++
	// Markdown styling is derived entirely from the theme, so every cached
	// renderer is stale now. dropRenderers must be called with mu held: it
	// takes no locks of its own precisely so it can be called from here.
	dropRenderers()
	defaultRenderer = newGlamourRenderer(themedStyleConfig(mdPalette, dark, syntaxStyle), 0)
}

// CurrentSyntaxStyle returns the active chroma style name.
func CurrentSyntaxStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	return syntaxStyle
}

// Palette returns the active diff palette.
func Palette() DiffPalette {
	mu.RLock()
	defer mu.RUnlock()
	return diffPalette
}

func buildDiffPalette(t *themes.Theme) DiffPalette {
	bg := t.Background
	green := t.Green
	red := t.Red
	return DiffPalette{
		AddFg:     green,
		DelFg:     red,
		AddBg:     themes.Mix(bg, green, 0.16),
		DelBg:     themes.Mix(bg, red, 0.14),
		PrefixAdd: themes.Mix(bg, green, 0.30),
		PrefixDel: themes.Mix(bg, red, 0.28),
		Hunk:      t.Blue,
		File:      t.Magenta,
		LineNum:   t.Muted,
		Context:   t.Subtext,
		WordDelBg: themes.Mix(bg, red, 0.32),
		WordAddBg: themes.Mix(bg, green, 0.34),
	}
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return hexColor(r, g, b)
}

func hexColor(r, g, b uint32) string {
	const digits = "0123456789abcdef"
	nib := func(v uint32, shift uint) byte {
		return digits[(v>>shift)&0xF]
	}
	buf := []byte{
		'#',
		nib(r, 12), nib(r, 8),
		nib(g, 12), nib(g, 8),
		nib(b, 12), nib(b, 8),
	}
	return string(buf)
}

// WordMarkers styles «deleted» / ‹inserted› word-diff markers inside a raw
// diff line, returning the line with ANSI styling applied.
func WordMarkers(line string, delStyle, addStyle lipgloss.Style) string {
	var result strings.Builder
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		switch runes[i] {
		case '«': // start of deleted word
			j := i + 1
			for j < len(runes) && runes[j] != '»' {
				j++
			}
			if j < len(runes) {
				result.WriteString(delStyle.Render(string(runes[i+1 : j])))
				i = j + 1
				continue
			}
		case '‹': // start of inserted word
			j := i + 1
			for j < len(runes) && runes[j] != '›' {
				j++
			}
			if j < len(runes) {
				result.WriteString(addStyle.Render(string(runes[i+1 : j])))
				i = j + 1
				continue
			}
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}
