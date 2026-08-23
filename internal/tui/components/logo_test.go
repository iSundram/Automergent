package components

import (
	"os"
	"testing"

	"github.com/iSundram/Automergent/internal/logo/render"
)

// safeRune reports whether r is drawable by any monospace font that
// covers Block Elements — the only assumption the wordmark may make
// about an unknown terminal. Anything outside this set (notably the
// Unicode 13 sextants at U+1FB00 and the Unicode 16 octants at
// U+1CD00) renders as tofu on stock fonts.
func safeRune(r rune) bool {
	return r == ' ' || (r >= 0x2580 && r <= 0x259F)
}

// TestDefaultGlyphSetIsFontSafe is the regression guard for the tofu
// bug: the default atlas must not emit code points that stock fonts
// lack. Quadrants satisfy this; octants and sextants do not.
func TestDefaultGlyphSetIsFontSafe(t *testing.T) {
	t.Setenv(glyphEnvVar, "")

	gs := bestGlyphSet()
	if gs != render.GlyphQuadrant {
		t.Errorf("default glyph set = %s, want quadrant (font-safe)", gs)
	}
}

// TestDefaultGlyphSetIgnoresTERM guards against reintroducing TERM
// sniffing: TERM advertises escape-sequence support, not font
// coverage, so no value of it may unlock the exotic atlases.
func TestDefaultGlyphSetIgnoresTERM(t *testing.T) {
	t.Setenv(glyphEnvVar, "")
	for _, term := range []string{
		"xterm-256color", "xterm-kitty", "wezterm", "alacritty",
		"screen.xterm-256color", "linux", "dumb", "",
	} {
		t.Run(term, func(t *testing.T) {
			t.Setenv("TERM", term)
			if gs := bestGlyphSet(); gs != render.GlyphQuadrant {
				t.Errorf("TERM=%q selected %s, want quadrant", term, gs)
			}
		})
	}
}

// TestGlyphEnvOverride confirms the documented opt-in still works, so
// users with legacy-computing fonts can restore the exact atlases.
func TestGlyphEnvOverride(t *testing.T) {
	for env, want := range map[string]render.GlyphSet{
		"octant":   render.GlyphAll,
		"sextant":  render.GlyphExtended,
		"blocks":   render.GlyphBlocks,
		"half":     render.GlyphHalf,
		"quadrant": render.GlyphQuadrant,
		"OCTANT":   render.GlyphAll,
		"  octant": render.GlyphAll,
	} {
		t.Setenv(glyphEnvVar, env)
		if got := bestGlyphSet(); got != want {
			t.Errorf("%s=%q selected %s, want %s", glyphEnvVar, env, got, want)
		}
	}

	// An unparseable value must fall back to the safe default rather
	// than to whatever the zero value happens to be.
	t.Setenv(glyphEnvVar, "not-a-glyph-set")
	if got := bestGlyphSet(); got != render.GlyphQuadrant {
		t.Errorf("invalid override selected %s, want quadrant", got)
	}
}

// TestLogoRendersFontSafeArt renders through the real component and
// checks the art the TUI would actually print.
func TestLogoRendersFontSafeArt(t *testing.T) {
	// The resolved set is cached in a sync.Once, so assert on the
	// process-wide value the TUI will use.
	if os.Getenv(glyphEnvVar) != "" {
		t.Skipf("%s is set in the environment", glyphEnvVar)
	}

	l := NewLogo(nil)
	l.SetWidth(75)
	art := l.View()
	if art == "" {
		t.Fatal("logo rendered empty art at width 75")
	}

	inSGR := false
	inked := 0
	for _, r := range art {
		switch {
		case r == 0x1b:
			inSGR = true
		case inSGR:
			// SGR parameters are digits/semicolons, terminated by 'm'.
			if r == 'm' {
				inSGR = false
			}
		case r == '\n':
		case !safeRune(r):
			t.Fatalf("rendered art contains untrusted rune U+%04X (%q)", r, r)
		default:
			if r != ' ' {
				inked++
			}
		}
	}
	if inked == 0 {
		t.Fatal("rendered art has no ink")
	}
}
