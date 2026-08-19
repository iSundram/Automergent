package components

import (
	"strings"

	"github.com/iSundram/Automergent/internal/logo/ansi"
	"github.com/iSundram/Automergent/internal/logo/render"
	"github.com/iSundram/Automergent/internal/logo/theme"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

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

// SetWidth re-renders the logo at the given width. Renders are
// expensive (the SVG is rasterized), so the art is cached and only
// rebuilt when the width actually changes.
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
// theme, matching what the logotui binary produces by default.
func (l *Logo) render() {
	th, err := theme.Lookup("brand")
	if err != nil {
		l.art = ""
		return
	}
	grid, err := render.Render(render.Options{
		Cols:   l.width,
		Ink:    th.Inker(theme.Dark),
		Glyphs: render.GlyphBlocks,
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