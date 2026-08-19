// Package ansi encodes a rendered grid as ANSI escape sequences and
// detects what the current terminal supports.
package ansi

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/iSundram/Automergent/internal/logo/render"
)

// ColorDepth is the color capability of an output terminal.
type ColorDepth int

const (
	// NoColor emits no escape sequences at all.
	NoColor ColorDepth = iota
	// ANSI16 uses the classic 16-color palette.
	ANSI16
	// ANSI256 uses the xterm 256-color cube.
	ANSI256
	// TrueColor uses 24-bit color.
	TrueColor
)

func (d ColorDepth) String() string {
	switch d {
	case NoColor:
		return "none"
	case ANSI16:
		return "16"
	case ANSI256:
		return "256"
	default:
		return "truecolor"
	}
}

// ParseDepth resolves a --color flag value.
func ParseDepth(s string) (ColorDepth, error) {
	switch strings.ToLower(s) {
	case "none", "off", "mono":
		return NoColor, nil
	case "16", "ansi":
		return ANSI16, nil
	case "256":
		return ANSI256, nil
	case "truecolor", "24bit", "true":
		return TrueColor, nil
	default:
		return 0, fmt.Errorf("unknown color depth %q (want: none, 16, 256, truecolor)", s)
	}
}

// DetectDepth infers the terminal's color support from the
// environment. It is deliberately conservative: when the environment
// is ambiguous it prefers 256, which virtually every modern terminal
// renders correctly.
func DetectDepth() ColorDepth {
	if os.Getenv("NO_COLOR") != "" {
		return NoColor
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return TrueColor
	}
	term := strings.ToLower(os.Getenv("TERM"))
	switch {
	case term == "" || term == "dumb":
		return NoColor
	case strings.Contains(term, "256color"):
		return ANSI256
	case strings.Contains(term, "color"):
		return ANSI16
	}
	return ANSI256
}

// Encoder writes grids as ANSI text.
type Encoder struct {
	Depth ColorDepth
	// Indent is a left margin applied to every line.
	Indent int
	// Trailing controls whether a reset + newline is emitted after
	// the final row.
	Trailing bool
}

// Encode renders the grid to a string of ANSI escapes.
//
// Only foreground colors are emitted, so the art composites onto
// whatever background the terminal already has. Runs of identical
// color reuse the active escape, which keeps the output compact.
func (e Encoder) Encode(g *render.Grid) string {
	var b strings.Builder
	// Rough preallocation: color runs dominate, so assume ~12 bytes
	// per cell.
	b.Grow(g.Cols * g.Rows * 12)

	const reset = "\x1b[0m"
	for y := 0; y < g.Rows; y++ {
		if e.Indent > 0 {
			b.WriteString(strings.Repeat(" ", e.Indent))
		}
		active := "" // currently applied escape
		// Trailing blanks carry no ink; buffer them so lines stay
		// short and do not paint the terminal's right margin.
		pending := 0
		for x := 0; x < g.Cols; x++ {
			c := g.At(x, y)
			if c.Glyph == " " {
				pending++
				continue
			}
			for ; pending > 0; pending-- {
				b.WriteByte(' ')
			}
			if e.Depth == NoColor {
				b.WriteString(c.Glyph)
				continue
			}
			esc := e.fg(c.FG)
			if esc != active {
				b.WriteString(esc)
				active = esc
			}
			b.WriteString(c.Glyph)
		}
		if active != "" {
			b.WriteString(reset)
		}
		if y < g.Rows-1 || e.Trailing {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// fg returns the SGR sequence setting c as the foreground color.
func (e Encoder) fg(c color.RGBA) string {
	switch e.Depth {
	case TrueColor:
		return "\x1b[38;2;" + itoa(int(c.R)) + ";" + itoa(int(c.G)) + ";" + itoa(int(c.B)) + "m"
	case ANSI256:
		return "\x1b[38;5;" + itoa(to256(c)) + "m"
	case ANSI16:
		return "\x1b[" + itoa(to16(c)) + "m"
	default:
		return ""
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// to256 maps an RGB color to the nearest xterm-256 index, choosing
// between the 6x6x6 color cube and the 24-step grey ramp.
func to256(c color.RGBA) int {
	// Grey ramp (indices 232..255) handles the neutral brand ink far
	// better than the cube, so test it explicitly.
	if c.R == c.G && c.G == c.B {
		if c.R < 8 {
			return 16
		}
		if c.R > 248 {
			return 231
		}
		return 232 + int(math.Round((float64(c.R)-8)/247*23))
	}
	cubeIdx := func(v uint8) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return int((float64(v) - 35) / 40)
	}
	r, g, b := cubeIdx(c.R), cubeIdx(c.G), cubeIdx(c.B)
	cube := 16 + 36*r + 6*g + b

	// Compare against the closest grey; pick whichever is nearer.
	grey := int(math.Round((float64(c.R)+float64(c.G)+float64(c.B))/3-8) / 247 * 23)
	if grey < 0 {
		grey = 0
	}
	if grey > 23 {
		grey = 23
	}
	greyIdx := 232 + grey
	greyVal := uint8(8 + grey*10)
	if dist(c, cubeColor(cube)) <= dist(c, color.RGBA{greyVal, greyVal, greyVal, 255}) {
		return cube
	}
	return greyIdx
}

// cubeColor returns the RGB value of a 6x6x6 cube index.
func cubeColor(idx int) color.RGBA {
	i := idx - 16
	level := func(v int) uint8 {
		if v == 0 {
			return 0
		}
		return uint8(55 + v*40)
	}
	return color.RGBA{level(i / 36), level((i / 6) % 6), level(i % 6), 255}
}

func dist(a, b color.RGBA) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	// Weighted to approximate perceptual distance.
	return 0.3*dr*dr + 0.59*dg*dg + 0.11*db*db
}

// ansi16 palette, in SGR code order (30-37 normal, 90-97 bright).
var palette16 = []struct {
	code int
	c    color.RGBA
}{
	{30, color.RGBA{0x00, 0x00, 0x00, 0xff}},
	{31, color.RGBA{0xaa, 0x00, 0x00, 0xff}},
	{32, color.RGBA{0x00, 0xaa, 0x00, 0xff}},
	{33, color.RGBA{0xaa, 0x55, 0x00, 0xff}},
	{34, color.RGBA{0x00, 0x00, 0xaa, 0xff}},
	{35, color.RGBA{0xaa, 0x00, 0xaa, 0xff}},
	{36, color.RGBA{0x00, 0xaa, 0xaa, 0xff}},
	{37, color.RGBA{0xaa, 0xaa, 0xaa, 0xff}},
	{90, color.RGBA{0x55, 0x55, 0x55, 0xff}},
	{91, color.RGBA{0xff, 0x55, 0x55, 0xff}},
	{92, color.RGBA{0x55, 0xff, 0x55, 0xff}},
	{93, color.RGBA{0xff, 0xff, 0x55, 0xff}},
	{94, color.RGBA{0x55, 0x55, 0xff, 0xff}},
	{95, color.RGBA{0xff, 0x55, 0xff, 0xff}},
	{96, color.RGBA{0x55, 0xff, 0xff, 0xff}},
	{97, color.RGBA{0xff, 0xff, 0xff, 0xff}},
}

// to16 maps an RGB color to the nearest of the 16 ANSI colors.
func to16(c color.RGBA) int {
	best, bestD := palette16[0].code, math.Inf(1)
	for _, p := range palette16 {
		if d := dist(c, p.c); d < bestD {
			best, bestD = p.code, d
		}
	}
	return best
}
