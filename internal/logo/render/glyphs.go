package render

import "math"

// Glyph sets trade terminal compatibility for effective resolution.
//
// A terminal cell is roughly twice as tall as it is wide. Block
// glyphs subdivide that cell into a small grid of "subpixels", each
// either inked or not; the cell then carries one foreground color for
// every inked subpixel. Fidelity at 80 columns, measured against the
// artwork (see the fidelity notes in README):
//
//	full block   1x1   IoU 0.67
//	half block   1x2   IoU 0.80
//	quadrant     2x2   IoU 0.86   exact: all 16 patterns exist
//	blocks       2x4   IoU 0.86   approximate solid atlas
//	sextant      2x3   ~exact     U+1FB00..1FB3B (Unicode 13)
//	octant       2x4   exact      U+1CD00..1CDE5 (Unicode 16)
//
// Quadrants are the sweet spot for universal support: they double
// horizontal resolution over half blocks and every pattern is exactly
// representable. Sextants and octants sample the cell finer still and
// represent every sampled pattern exactly, but need a font with the
// Symbols for Legacy Computing blocks; terminals or fonts without
// them render tofu, so callers should gate them on terminal
// capability (see components.logo).

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
	// Block Elements and shade glyphs. Renders everywhere.
	GlyphBlocks
	// GlyphExtended samples at 2x3 and renders exact sextants, falling
	// back to half/quadrant blocks for the four patterns those glyphs
	// already cover.
	GlyphExtended
	// GlyphAll samples at 2x4 and renders exact octants — the most
	// comprehensive block atlas — falling back to Block Elements for
	// the 26 patterns those glyphs already cover.
	GlyphAll
	// GlyphSextant is an explicit alias of GlyphExtended.
	GlyphSextant
	// GlyphOctant is an explicit alias of GlyphAll.
	GlyphOctant
)

// ParseGlyphSet resolves a glyph set name.
func ParseGlyphSet(s string) (GlyphSet, bool) {
	switch s {
	case "quadrant", "quad", "":
		return GlyphQuadrant, true
	case "half":
		return GlyphHalf, true
	case "braille":
		return GlyphBraille, true
	case "blocks", "block", "standard":
		return GlyphBlocks, true
	case "sextant", "sex", "extended", "mixed":
		return GlyphExtended, true
	case "octant", "oct", "all", "best", "auto":
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
	case GlyphExtended, GlyphSextant:
		return "sextant"
	case GlyphAll, GlyphOctant:
		return "octant"
	default:
		return "quadrant"
	}
}

// dims returns the subpixel grid of one cell: columns and rows.
func (g GlyphSet) dims() (w, h int) {
	switch g {
	case GlyphHalf:
		return 1, 2
	case GlyphBraille, GlyphBlocks, GlyphAll, GlyphOctant:
		return 2, 4
	case GlyphExtended, GlyphSextant:
		return 2, 3
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

// sextantLUT maps a 2x3 pattern (bit y*2+x set when that subpixel is
// inked, rows top to bottom) to its BLOCK SEXTANT rune. The 60
// sextants at U+1FB00..U+1FB3B cover every pattern except the four
// that legacy Block Elements already provide: empty (space), left
// column (▌), right column (▐) and full (█).
//
// Code points are assigned to the remaining patterns in ascending
// binary order, so rune(U+1FB00 + n) for n = pattern minus the number
// of excluded patterns below it. Spot checks against the Unicode
// names list: pattern 0b000001 -> U+1FB00 SEXTANT-1,
// 0b000111 -> U+1FB06 SEXTANT-123, 0b001000 -> U+1FB07 SEXTANT-4.
var sextantLUT = func() [64]string {
	excluded := map[uint8]string{
		0b000000: " ",
		0b010101: "▌",
		0b101010: "▐",
		0b111111: "█",
	}
	var lut [64]string
	off := 0
	for b := 0; b < 64; b++ {
		if g, ok := excluded[uint8(b)]; ok {
			lut[b] = g
			continue
		}
		lut[b] = string(rune(0x1FB00 + off))
		off++
	}
	return lut
}()

// octantSpecial maps the 26 octant patterns whose geometry is already
// carried by existing characters (Block Elements, quadrants, upper
// fractional blocks) to those characters. Every other pattern has its
// own BLOCK OCTANT rune in the contiguous run U+1CD00..U+1CDE5.
//
// Bit n of the pattern is octant cell n+1, numbered row-major across
// two columns and four rows. Spot checks against the Unicode names
// list: 0b000100 -> U+1CD00 OCTANT-3, 0b001000 -> U+1CD03 OCTANT-4,
// 0b0010000 -> U+1CD09 OCTANT-5, 0b01010101 -> U+258C LEFT HALF.
var octantSpecial = map[uint8]rune{
	0b00000000: ' ',     // space (spec uses U+00A0; identical on screen)
	0b00000001: 0x1CEA8, // octant-1
	0b00000010: 0x1CEAB, // octant-2
	0b00000011: 0x1FB82, // upper quarter
	0b00000101: 0x2598,  // ▘ quadrant upper left
	0b00001010: 0x259D,  // ▝ quadrant upper right
	0b00001111: 0x2580,  // ▀ upper half
	0b00010100: 0x1FBE6, // lower left sixth pair
	0b00101000: 0x1FBE7, // lower right sixth pair
	0b00111111: 0x1FB85, // upper three quarters
	0b01000000: 0x1CEA3, // octant-7
	0b01010000: 0x2596,  // ▖ quadrant lower left
	0b01010101: 0x258C,  // ▌ left half
	0b01011010: 0x259E,  // ▞ quadrant diagonal
	0b01011111: 0x259B,  // ▛ quadrant upper three
	0b10000000: 0x1CEA0, // octant-8
	0b10100000: 0x2597,  // ▗ quadrant lower right
	0b10100101: 0x259A,  // ▚ quadrant anti-diagonal
	0b10101010: 0x2590,  // ▐ right half
	0b10101111: 0x259C,  // ▜ quadrant upper right three
	0b11000000: 0x2582,  // ▂ lower quarter
	0b11110000: 0x2584,  // ▄ lower half
	0b11110101: 0x2599,  // ▙ quadrant lower three
	0b11111010: 0x259F,  // ▟ quadrant lower right three
	0b11111100: 0x2586,  // ▆ lower three quarters
	0b11111111: 0x2588,  // █ full block
}

// octantLUT maps every 2x4 pattern to an exact rune: a BLOCK OCTANT
// for the 230 patterns with dedicated code points, one of the legacy
// stand-ins above otherwise. Built once at startup so glyph selection
// is a single slice index at render time.
var octantLUT = func() [256]string {
	special := make([]uint8, 0, len(octantSpecial))
	for b := range octantSpecial {
		special = append(special, b)
	}
	sortBytes(special)

	var lut [256]string
	off := 0
	si := 0
	for b := 0; b < 256; b++ {
		if si < len(special) && special[si] == uint8(b) {
			si++
			continue
		}
		lut[b] = string(rune(0x1CD00 + off))
		off++
	}
	for b, r := range octantSpecial {
		lut[b] = string(r)
	}
	return lut
}()

// glyphFor converts a subpixel pattern into a single character. The
// pattern is row-major with bit (y*w + x) set when that subpixel is
// inked. Sextants and octants resolve through total lookup tables;
// every pattern of their sampling grid is exactly representable.
func (g GlyphSet) glyphFor(pattern uint16) string {
	switch g {
	case GlyphHalf:
		return halfGlyphs[pattern&0b11]
	case GlyphBraille:
		return brailleGlyph(pattern)
	case GlyphBlocks:
		return nearestBlockGlyph(pattern)
	case GlyphExtended, GlyphSextant:
		return sextantLUT[pattern&0b111111]
	case GlyphAll, GlyphOctant:
		return octantLUT[pattern&0xff]
	default:
		return quadrantGlyphs[pattern&0b1111]
	}
}

// standardBlockGlyphs contains the solid Block Elements plus the three
// shade characters. Masks use the same row-major 2x4 layout as
// braille. The fractional glyphs are deliberately represented as ideal
// geometry; the terminal font supplies the final antialiasing.
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

// nearestBlockGlyph picks the standard Block Element closest to a 2x4
// pattern. It is only used by GlyphBlocks; the exact atlases resolve
// through their lookup tables instead.
func nearestBlockGlyph(pattern uint16) string {
	density := float64(bitsOnes(pattern)) / 8
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
	return best
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

func sortBytes(b []uint8) {
	for i := 1; i < len(b); i++ {
		for j := i; j > 0 && b[j] < b[j-1]; j-- {
			b[j], b[j-1] = b[j-1], b[j]
		}
	}
}
