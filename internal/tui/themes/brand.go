package themes

import (
	"image/color"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

// brandGlyphEnvVar overrides the brand mark. Font coverage cannot be probed
// from inside a terminal — a missing or misdrawn glyph still reports a
// successful cursor advance — so there is no signal to detect on and this is the
// escape hatch when a particular font draws the default badly.
const brandGlyphEnvVar = "AUTOMERGENT_BRAND_GLYPH"

// defaultBrandGlyph is a four-pointed star with U+FE0E, the text-presentation
// variation selector, pinned to it.
//
// The selector is not decoration. U+2726 is East-Asian-Ambiguous, and a font
// stack that resolves it through an emoji face draws a double-width glyph into
// the single cell the terminal advanced — which is the star appearing clipped
// rather than whole. VS-15 forces the text face, and it is zero-width, so the
// mark still measures one cell.
const defaultBrandGlyph = "✦︎"

// BrandName is the wordmark text that follows the mark in bars wide enough to
// hold it.
const BrandName = "AUTOMERGENT"

var (
	brandGlyphOnce sync.Once
	brandGlyph     string
)

// BrandGlyph is the mark that stands in for the product name.
//
// A four-pointed star rather than the older lozenge: the lozenge read as a list
// bullet, and the conversation label is the one place the brand appears with no
// text beside it, so the glyph has to carry the identity alone.
func BrandGlyph() string {
	brandGlyphOnce.Do(func() {
		brandGlyph = defaultBrandGlyph
		if v := strings.TrimSpace(os.Getenv(brandGlyphEnvVar)); v != "" {
			brandGlyph = v
		}
	})
	return brandGlyph
}

// BrandColor is the mark's ink: the theme's own blue, so the glyph reads as the
// same brand in every palette without ever being a color the theme does not
// contain.
func (t *Theme) BrandColor() color.Color {
	if t == nil {
		return Catppuccin().Blue
	}
	return t.Blue
}

// BrandMark renders the brand glyph followed by one space.
//
// The space is part of the mark, not the caller's business: it guarantees the
// cell to the right of the star is empty, so a font whose glyph overruns its
// advance width has somewhere to overrun into instead of being clipped by the
// next character. Callers append their text directly — adding another space of
// their own is what produced the doubled gap.
//
// Deliberately not bold. Terminals without a bold face for a symbol synthesize
// one by smearing the glyph horizontally, which pushes a star's right point past
// the cell boundary and gets it cut. The color carries the mark on its own.
func (s *Styles) BrandMark() string {
	return lipgloss.NewStyle().
		Foreground(s.T.BrandColor()).
		Render(BrandGlyph()) + " "
}
