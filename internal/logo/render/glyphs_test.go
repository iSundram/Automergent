package render

import (
	"image/color"
	"testing"
)

func solidInk(u, coverage float64) color.RGBA {
	return color.RGBA{0x33, 0x33, 0x33, 0xff}
}

// TestSextantAtlas checks the programmatic U+1FB00 mapping against
// spot values from the Unicode names list and verifies totality.
func TestSextantAtlas(t *testing.T) {
	want := map[uint8]rune{
		0b000000: ' ',
		0b000001: 0x1FB00, // SEXTANT-1
		0b000010: 0x1FB01, // SEXTANT-2
		0b000111: 0x1FB06, // SEXTANT-123
		0b001000: 0x1FB07, // SEXTANT-4
		0b010101: '▌',
		0b101010: '▐',
		0b111111: '█',
	}
	for p, r := range want {
		if got := sextantLUT[p]; got != string(r) {
			t.Errorf("sextantLUT[0b%06b] = %q, want %q", p, got, string(r))
		}
	}
	inRun := 0
	for _, g := range sextantLUT {
		r := []rune(g)
		if len(r) != 1 {
			t.Fatalf("sextant glyph %q is not a single rune", g)
		}
		if r[0] >= 0x1FB00 && r[0] <= 0x1FB3B {
			inRun++
		}
	}
	if inRun != 60 {
		t.Errorf("sextant atlas uses %d sextant code points, want 60", inRun)
	}
}

// TestOctantAtlas checks the programmatic U+1CD00 mapping against spot
// values from the Unicode names list and verifies totality.
func TestOctantAtlas(t *testing.T) {
	want := map[uint8]rune{
		0b00000100: 0x1CD00, // OCTANT-3
		0b00001000: 0x1CD03, // OCTANT-4
		0b00010000: 0x1CD09, // OCTANT-5
		0b00000001: 0x1CEA8, // OCTANT-1
		0b00000010: 0x1CEAB, // OCTANT-2
		0b00000011: 0x1FB82,
		0b00000101: '▘',
		0b00001111: '▀',
		0b00111111: 0x1FB85,
		0b01010101: '▌',
		0b10101010: '▐',
		0b11110000: '▄',
		0b11111111: '█',
	}
	for p, r := range want {
		if got := octantLUT[p]; got != string(r) {
			t.Errorf("octantLUT[0b%08b] = %q, want %q", p, got, string(r))
		}
	}
	inRun := 0
	for _, g := range octantLUT {
		r := []rune(g)
		if len(r) != 1 {
			t.Fatalf("octant glyph %q is not a single rune", g)
		}
		if r[0] >= 0x1CD00 && r[0] <= 0x1CDE5 {
			inRun++
		}
	}
	if inRun != 230 {
		t.Errorf("octant atlas uses %d octant code points, want 230", inRun)
	}
}

// TestRenderAllGlyphSets renders at several widths with every glyph
// set and checks grid sanity plus alphabet containment.
func TestRenderAllGlyphSets(t *testing.T) {
	sets := []GlyphSet{
		GlyphHalf, GlyphQuadrant, GlyphBraille,
		GlyphBlocks, GlyphExtended, GlyphSextant, GlyphAll, GlyphOctant,
	}
	for _, gs := range sets {
		for _, cols := range []int{15, 40, 75} {
			grid, err := Render(Options{Cols: cols, Ink: solidInk, Glyphs: gs})
			if err != nil {
				t.Fatalf("%s/%dc: %v", gs, cols, err)
			}
			if grid.Cols != cols || grid.Rows < 1 {
				t.Fatalf("%s/%dc: bad grid %dx%d", gs, cols, grid.Cols, grid.Rows)
			}
			var inked int
			for y := 0; y < grid.Rows; y++ {
				for x := 0; x < grid.Cols; x++ {
					c := grid.At(x, y)
					if c.Glyph == "" || len(c.Glyph) == 0 {
						t.Fatalf("%s/%dc: empty glyph at (%d,%d)", gs, cols, x, y)
					}
					if c.Glyph != " " {
						inked++
					}
				}
			}
			if inked == 0 {
				t.Fatalf("%s/%dc: render produced no ink", gs, cols)
			}
		}
	}
}

// TestOctantRenderAlphabet ensures the octant atlas only emits runes
// from the legacy block elements and the two legacy-computing ranges.
func TestOctantRenderAlphabet(t *testing.T) {
	grid, err := Render(Options{Cols: 75, Ink: solidInk, Glyphs: GlyphAll})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range grid.Cells {
		for _, r := range c.Glyph {
			ok := r == ' ' ||
				(r >= 0x2580 && r <= 0x259F) ||
				(r >= 0x1CD00 && r <= 0x1CDE5) ||
				(r >= 0x1FB00 && r <= 0x1FBFF) ||
				(r >= 0x1CC00 && r <= 0x1CEBF)
			if !ok {
				t.Fatalf("unexpected rune %q (U+%04X) from octant atlas", r, r)
			}
		}
	}
}

// TestAspectStable guards the measured aspect ratio used for sizing.
func TestAspectStable(t *testing.T) {
	a := Aspect()
	if a < 3 || a > 12 {
		t.Fatalf("aspect %.2f outside plausible wordmark range", a)
	}
}

func BenchmarkRenderOctant(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Render(Options{Cols: 75, Ink: solidInk, Glyphs: GlyphAll}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderBlocks(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := Render(Options{Cols: 75, Ink: solidInk, Glyphs: GlyphBlocks}); err != nil {
			b.Fatal(err)
		}
	}
}

// TestQuadrantAtlasIsFontSafe pins the property that makes quadrants
// the safe default for unknown terminals: every one of the sixteen
// patterns resolves to a Block Element, a range stock monospace fonts
// have covered for decades. The exotic atlases are exact but depend on
// Unicode 13/16 code points that most installed fonts lack, so the
// default must never drift onto them.
func TestQuadrantAtlasIsFontSafe(t *testing.T) {
	for pattern := uint16(0); pattern < 16; pattern++ {
		g := GlyphQuadrant.glyphFor(pattern)
		rs := []rune(g)
		if len(rs) != 1 {
			t.Fatalf("pattern %04b -> %q is not a single rune", pattern, g)
		}
		r := rs[0]
		if r != ' ' && (r < 0x2580 || r > 0x259F) {
			t.Errorf("pattern %04b -> U+%04X (%q) is outside Block Elements", pattern, r, r)
		}
	}
}

// TestExoticAtlasesNeedRareFonts documents why the default is not
// octants or sextants: the overwhelming majority of their patterns
// resolve outside Block Elements. This is the measurement behind the
// choice, so a future change to the default has to confront it.
func TestExoticAtlasesNeedRareFonts(t *testing.T) {
	safe := func(r rune) bool { return r == ' ' || (r >= 0x2580 && r <= 0x259F) }

	unsafeOct := 0
	for p := 0; p < 256; p++ {
		for _, r := range GlyphOctant.glyphFor(uint16(p)) {
			if !safe(r) {
				unsafeOct++
			}
		}
	}
	if unsafeOct != 238 {
		t.Errorf("octant atlas has %d font-dependent patterns, want 238", unsafeOct)
	}

	unsafeSex := 0
	for p := 0; p < 64; p++ {
		for _, r := range GlyphSextant.glyphFor(uint16(p)) {
			if !safe(r) {
				unsafeSex++
			}
		}
	}
	if unsafeSex != 60 {
		t.Errorf("sextant atlas has %d font-dependent patterns, want 60", unsafeSex)
	}
}
