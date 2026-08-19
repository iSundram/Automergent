package render

import "math"

// Glyph sets trade terminal compatibility for effective resolution.
//
// A terminal cell is roughly twice as tall as it is wide. Block
// glyphs subdivide that cell into a small grid of "subpixels", each
// either inked or not; the cell then carries one foreground color for
// every inked subpixel. Measured against the artwork at 80 columns
// (see the fidelity notes in README), the sets score:
//
//	full block   1x1   IoU 0.67
//	half block   1x2   IoU 0.80
//	quadrant     2x2   IoU 0.86   <- default
//	sextant      2x3   IoU 0.86
//	braille      2x4   IoU 0.89
//
// Quadrants are the sweet spot: they double horizontal resolution
// over half blocks, which is what opens up the enclosed counters in
// e, g and o, and they are drawn correctly by every font that already
// handles the half blocks. Braille buys a little more detail but
// renders as a dot matrix rather than solid strokes, and many fonts
// space it inconsistently.

// GlyphSet identifies a cell subdivision strategy.
type GlyphSet int

const (
	// GlyphQuadrant divides each cell into 2x2 subpixels.
	GlyphQuadrant GlyphSet = iota
	// GlyphHalf divides each cell into 1x2 subpixels (top/bottom).
	GlyphHalf
	// GlyphBraille divides each cell into 2x4 subpixels.
	GlyphBraille
	// GlyphBlocks samples at 2x4 and selects from the standard Unicode
	// Block Elements and shade glyphs.
	GlyphBlocks
	// GlyphExtended includes the standard block atlas and braille glyphs.
	GlyphExtended
	// GlyphAll is the highest-resolution mixed atlas available locally.
	GlyphAll
)

// ParseGlyphSet resolves a --glyphs flag value.
func ParseGlyphSet(s string) (GlyphSet, bool) {
	switch s {
	case "quadrant", "quad", "":
		return GlyphQuadrant, true
	case "auto":
		return GlyphBlocks, true
	case "half":
		return GlyphHalf, true
	case "braille":
		return GlyphBraille, true
	case "blocks", "block", "standard":
		return GlyphBlocks, true
	case "extended", "mixed":
		return GlyphExtended, true
	case "all", "best":
		return GlyphAll, true
	}
	return 0, false
}

// String implements fmt.Stringer.
func (g GlyphSet) String() string {
	switch g {
	case GlyphHalf:
		return "half"
	case GlyphBraille:
		return "braille"
	case GlyphBlocks:
		return "blocks"
	case GlyphExtended:
		return "extended"
	case GlyphAll:
		return "all"
	default:
		return "quadrant"
	}
}

// dims returns the subpixel grid of one cell: columns and rows.
func (g GlyphSet) dims() (w, h int) {
	switch g {
	case GlyphHalf:
		return 1, 2
	case GlyphBraille:
		return 2, 4
	case GlyphBlocks, GlyphExtended, GlyphAll:
		return 2, 4
	default:
		return 2, 2
	}
}

// quadrantGlyphs is indexed by a 4-bit pattern where bit 0 is the
// top-left subpixel, bit 1 top-right, bit 2 bottom-left and bit 3
// bottom-right. All sixteen combinations exist as single characters,
// so any 2x2 pattern is representable exactly.
var quadrantGlyphs = [16]string{
	0b0000: " ",
	0b0001: "▘",
	0b0010: "▝",
	0b0011: "▀",
	0b0100: "▖",
	0b0101: "▌",
	0b0110: "▞",
	0b0111: "▛",
	0b1000: "▗",
	0b1001: "▚",
	0b1010: "▐",
	0b1011: "▜",
	0b1100: "▄",
	0b1101: "▙",
	0b1110: "▟",
	0b1111: "█",
}

// halfGlyphs is indexed by a 2-bit pattern: bit 0 top, bit 1 bottom.
var halfGlyphs = [4]string{
	0b00: " ",
	0b01: "▀",
	0b10: "▄",
	0b11: "█",
}

// brailleBits maps a (column, row) position in the 2x4 subpixel grid
// to its bit in the Unicode braille pattern block. The braille dot
// numbering is column-major and puts the fourth row last, so it is
// easier to spell out than to compute.
var brailleBits = [2][4]rune{
	{0x01, 0x02, 0x04, 0x40}, // left column:  dots 1,2,3,7
	{0x08, 0x10, 0x20, 0x80}, // right column: dots 4,5,6,8
}

// glyphFor converts a subpixel pattern into a single character. The
// pattern is row-major with bit (y*w + x) set when that subpixel is
// inked.
func (g GlyphSet) glyphFor(pattern uint16, density float64) string {
	switch g {
	case GlyphHalf:
		return halfGlyphs[pattern&0b11]
	case GlyphBraille:
		return brailleGlyph(pattern)
	case GlyphBlocks:
		return nearestBlockGlyph(pattern, density, false)
	case GlyphExtended, GlyphAll:
		return nearestBlockGlyph(pattern, density, true)
	default:
		return quadrantGlyphs[pattern&0b1111]
	}
}

// standardBlockGlyphs contains the solid Block Elements plus the three
// shade characters. Masks use the same row-major 2x4 layout as braille.
// The fractional glyphs are deliberately represented as ideal geometry;
// the terminal font supplies the final antialiasing.
var standardBlockGlyphs = []struct {
	glyph string
	mask  uint16
	fill  float64
}{
	{glyph: " ", mask: 0, fill: 0},
	{glyph: "█", mask: 0xff, fill: 1},
	{glyph: "▀", mask: 0x0f, fill: .5},
	{glyph: "▄", mask: 0xf0, fill: .5},
	{glyph: "▌", mask: 0x55, fill: .5},
	{glyph: "▐", mask: 0xaa, fill: .5},
	{glyph: "▏", mask: 0x55, fill: .125},
	{glyph: "▎", mask: 0x55, fill: .25},
	{glyph: "▍", mask: 0x55, fill: .375},
	{glyph: "▕", mask: 0xaa, fill: .125},
	{glyph: "▔", mask: 0x03, fill: .125},
	{glyph: "▁", mask: 0xc0, fill: .125},
	{glyph: "▂", mask: 0xf0, fill: .25},
	{glyph: "▃", mask: 0xfc, fill: .375},
	{glyph: "▅", mask: 0xfc, fill: .625},
	{glyph: "▆", mask: 0xfc, fill: .75},
	{glyph: "▇", mask: 0xfe, fill: .875},
	{glyph: "▘", mask: 0x05, fill: .25},
	{glyph: "▝", mask: 0x0a, fill: .25},
	{glyph: "▖", mask: 0x50, fill: .25},
	{glyph: "▗", mask: 0xa0, fill: .25},
	{glyph: "▞", mask: 0x5a, fill: .5},
	{glyph: "▛", mask: 0x5f, fill: .75},
	{glyph: "▚", mask: 0xa5, fill: .5},
	{glyph: "▜", mask: 0xaf, fill: .75},
	{glyph: "▙", mask: 0xf5, fill: .75},
	{glyph: "▟", mask: 0xfa, fill: .75},
	{glyph: "░", mask: 0x55, fill: .25},
	{glyph: "▒", mask: 0x99, fill: .5},
	{glyph: "▓", mask: 0xee, fill: .75},
}

func brailleGlyph(pattern uint16) string {
	var r rune
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			if pattern&(1<<(y*2+x)) != 0 {
				r |= brailleBits[x][y]
			}
		}
	}
	if r == 0 {
		return " "
	}
	return string(rune(0x2800) + r)
}

func nearestBlockGlyph(pattern uint16, density float64, includeBraille bool) string {
	best, bestScore := " ", math.Inf(1)
	for _, candidate := range standardBlockGlyphs {
		d := float64(bitDistance(pattern, candidate.mask))
		score := d + 4*math.Abs(density-candidate.fill)
		// Prefer solid blocks over textured shades when the geometry is
		// equally good. This keeps the logo from turning into stippling.
		if candidate.glyph == "░" || candidate.glyph == "▒" || candidate.glyph == "▓" {
			score += .2
		}
		if score < bestScore {
			best, bestScore = candidate.glyph, score
		}
	}
	if includeBraille && pattern != 0 {
		// Braille exactly preserves the sampled pattern. It is considered
		// only when the solid atlas cannot represent it closely.
		if bitDistance(pattern, blockMask(best)) > 1 {
			return brailleGlyph(pattern)
		}
	}
	return best
}

func blockMask(glyph string) uint16 {
	for _, candidate := range standardBlockGlyphs {
		if candidate.glyph == glyph {
			return candidate.mask
		}
	}
	return 0
}

func bitDistance(a, b uint16) int {
	return bitsOnes(a ^ b)
}

func bitsOnes(v uint16) int {
	n := 0
	for v != 0 {
		v &= v - 1
		n++
	}
	return n
}

// blank reports whether a pattern has no inked subpixels.
func (g GlyphSet) blank(pattern uint16) bool { return pattern == 0 }
