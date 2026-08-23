package render

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

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
)

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
	// Width-keyed markdown renderers are cached by "dark|width" so stale
	// entries are simply never hit again after a dark/light flip.
	defaultRenderer = newGlamourRenderer(tameStyle(baseStyleConfig(dark)), 0)
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
