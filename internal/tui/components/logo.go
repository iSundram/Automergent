package components

import (
	"os"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/logo/ansi"
	"github.com/iSundram/Automergent/internal/logo/render"
	"github.com/iSundram/Automergent/internal/logo/theme"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// glyphEnvVar overrides the glyph set used for the wordmark. Accepts
// any render.ParseGlyphSet name: half, quadrant, braille, blocks,
// sextant (extended) or octant (all).
const glyphEnvVar = "AUTOMERGENT_LOGO_GLYPHS"

// bestGlyphSet resolves the highest-fidelity block atlas this terminal
// can be trusted to render.
//
// Quadrants are the default because they are the finest subdivision
// whose every pattern is a Block Element (U+2580..U+259F) — a range
// present in essentially every monospace font shipped in the last
// three decades. Sextants (Unicode 13) and octants (Unicode 16) sample
// the cell finer and are exact, but 238 of the 256 octant patterns
// live in the Symbols for Legacy Computing ranges that almost no
// installed font covers yet; selecting them unconditionally renders
// the wordmark as a field of tofu.
//
// Font coverage cannot be probed from inside the terminal: a missing
// glyph still occupies one column, so cursor-position queries report
// success for characters that draw as a replacement box. There is no
// signal to detect on, hence a safe default plus the opt-in
// environment variable above for terminals known to have the fonts.
func bestGlyphSet() render.GlyphSet {
	if v := strings.TrimSpace(os.Getenv(glyphEnvVar)); v != "" {
		if gs, ok := render.ParseGlyphSet(strings.ToLower(v)); ok {
			return gs
		}
	}
	return render.GlyphQuadrant
}

var (
	glyphOnce   sync.Once
	resolvedSet render.GlyphSet
)

func resolveGlyphSet() render.GlyphSet {
	glyphOnce.Do(func() { resolvedSet = bestGlyphSet() })
	return resolvedSet
}

// Logo renders the Automergent terminal wordmark. The art is
// regenerated whenever the width changes so it always fits the
// current terminal size.
type Logo struct {
	styles *themes.Styles
	width  int
	art    string
}

// NewLogo creates a Logo component.
func NewLogo(styles *themes.Styles) Logo {
	return Logo{styles: styles}
}

// SetWidth re-renders the logo at the given width. The rasterized ink
// plane is cached process-wide, so a re-render is a cheap downsample
// plus table lookups; the art is only rebuilt when the width changes.
func (l *Logo) SetWidth(w int) {
	if w == l.width || w <= 0 {
		return
	}
	l.width = w
	l.render()
}

// Width reports the width the current art was rendered at.
func (l Logo) Width() int { return l.width }

// render rasterizes the wordmark at l.width using the default brand
// theme and the terminal's best glyph atlas.
func (l *Logo) render() {
	th, err := theme.Lookup("brand")
	if err != nil {
		l.art = ""
		return
	}
	grid, err := render.Render(render.Options{
		Cols:   l.width,
		Ink:    th.Inker(theme.Dark),
		Glyphs: resolveGlyphSet(),
	})
	if err != nil {
		l.art = ""
		return
	}
	enc := ansi.Encoder{Depth: ansi.ANSI256, Trailing: true}
	l.art = enc.Encode(grid)
}

// View returns the rendered wordmark, or an empty string if it has
// not been sized yet.
func (l Logo) View() string {
	return l.art
}

// Height reports the rendered art height in terminal rows.
func (l Logo) Height() int {
	return strings.Count(l.art, "\n") + 1
}
