// Package theme provides the ink treatments for the wordmark: the
// faithful brand color, its light counterpart for dark terminals, and
// the optional --theme color ramps.
package theme

import (
	"fmt"
	"image/color"
	"math"
	"sort"
)

// Brand is the wordmark's authored ink: #333333, read from every
// gradient stop in the source SVG.
var Brand = color.RGBA{0x33, 0x33, 0x33, 0xff}

// BrandLight is the auto-invert counterpart used on dark terminals.
// The brand ink sits at ~20% relative luminance against white; this
// mirrors that contrast against a dark background rather than using
// pure white, so the mark keeps its soft, non-glaring weight.
var BrandLight = color.RGBA{0xd6, 0xd6, 0xd6, 0xff}

// Background is the surface the ink is composited against. Because
// the renderer never emits background color codes, partial coverage
// is blended toward this assumed color to fake antialiasing.
type Background int

const (
	// Dark assumes a dark terminal (blend toward near-black).
	Dark Background = iota
	// Light assumes a light terminal (blend toward near-white).
	Light
)

func (b Background) color() color.RGBA {
	if b == Light {
		return color.RGBA{0xff, 0xff, 0xff, 0xff}
	}
	return color.RGBA{0x00, 0x00, 0x00, 0xff}
}

// String implements fmt.Stringer.
func (b Background) String() string {
	if b == Light {
		return "light"
	}
	return "dark"
}

// A Theme is a named ink treatment.
type Theme struct {
	Name string
	Desc string
	// stops is the color ramp across the wordmark width. A single
	// stop is a solid color.
	stops []color.RGBA
	// follows reports whether the theme adapts to the background.
	follows bool
}

var registry = map[string]Theme{
	"brand": {
		Name:    "brand",
		Desc:    "faithful #333333 (auto-inverts to light ink on dark terminals)",
		stops:   nil, // resolved per-background
		follows: true,
	},
	"mono": {
		Name:  "mono",
		Desc:  "strict #333333, never inverted",
		stops: []color.RGBA{Brand},
	},
	"aurora": {
		Name: "aurora",
		Desc: "teal -> cyan -> violet ramp",
		stops: []color.RGBA{
			{0x2d, 0xd4, 0xa8, 0xff},
			{0x38, 0xbd, 0xf8, 0xff},
			{0xa7, 0x8b, 0xfa, 0xff},
		},
	},
	"solar": {
		Name: "solar",
		Desc: "amber -> coral -> magenta ramp",
		stops: []color.RGBA{
			{0xfb, 0xbf, 0x24, 0xff},
			{0xfb, 0x71, 0x85, 0xff},
			{0xe8, 0x3d, 0xa8, 0xff},
		},
	},
	"neon": {
		Name: "neon",
		Desc: "high-voltage green -> cyan -> purple",
		stops: []color.RGBA{
			{0x39, 0xff, 0x14, 0xff},
			{0x00, 0xe5, 0xff, 0xff},
			{0xb4, 0x1c, 0xff, 0xff},
		},
	},
	"ember": {
		Name: "ember",
		Desc: "deep red -> orange -> gold",
		stops: []color.RGBA{
			{0xdc, 0x26, 0x26, 0xff},
			{0xf9, 0x73, 0x16, 0xff},
			{0xfa, 0xcc, 0x15, 0xff},
		},
	},
}

// Names returns the registered theme names, sorted, with "brand"
// first since it is the default.
func Names() []string {
	rest := make([]string, 0, len(registry))
	for n := range registry {
		if n != "brand" {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	return append([]string{"brand"}, rest...)
}

// Describe returns a human-readable listing of the themes.
func Describe() string {
	out := ""
	for _, n := range Names() {
		out += fmt.Sprintf("    %-8s %s\n", n, registry[n].Desc)
	}
	return out
}

// Lookup resolves a theme by name.
func Lookup(name string) (Theme, error) {
	t, ok := registry[name]
	if !ok {
		return Theme{}, fmt.Errorf("unknown theme %q (available: %v)", name, Names())
	}
	return t, nil
}

// Inker returns a function mapping horizontal position and coverage
// to a foreground color, compositing against bg.
//
// The returned function matches render.Inker.
func (t Theme) Inker(bg Background) func(u, coverage float64) color.RGBA {
	stops := t.stops
	if t.follows {
		// "brand" adapts: exact #333333 on light, its light
		// counterpart on dark.
		if bg == Light {
			stops = []color.RGBA{Brand}
		} else {
			stops = []color.RGBA{BrandLight}
		}
	}
	surface := bg.color()
	return func(u, coverage float64) color.RGBA {
		ink := sample(stops, u)
		// Blend toward the surface by coverage: this is the
		// antialiasing, since no background codes are emitted.
		return blend(surface, ink, clamp01(coverage))
	}
}

// IsFaithful reports whether the theme preserves the authored brand
// color (possibly inverted for legibility).
func (t Theme) IsFaithful() bool { return t.follows || len(t.stops) == 1 }

// sample evaluates the ramp at u in [0,1].
func sample(stops []color.RGBA, u float64) color.RGBA {
	switch len(stops) {
	case 0:
		return Brand
	case 1:
		return stops[0]
	}
	u = clamp01(u)
	// Position u across len(stops)-1 segments.
	seg := u * float64(len(stops)-1)
	i := int(seg)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	return blend(stops[i], stops[i+1], seg-float64(i))
}

// blend mixes a toward b by t in linear-light space, which avoids the
// muddy midtones of naive sRGB interpolation.
func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		encode(decode(a.R) + t*(decode(b.R)-decode(a.R))),
		encode(decode(a.G) + t*(decode(b.G)-decode(a.G))),
		encode(decode(a.B) + t*(decode(b.B)-decode(a.B))),
		0xff,
	}
}

// decode converts an sRGB byte to linear light.
func decode(v uint8) float64 {
	f := float64(v) / 255
	if f <= 0.04045 {
		return f / 12.92
	}
	return math.Pow((f+0.055)/1.055, 2.4)
}

// encode converts linear light back to an sRGB byte.
func encode(f float64) uint8 {
	f = clamp01(f)
	var s float64
	if f <= 0.0031308 {
		s = f * 12.92
	} else {
		s = 1.055*math.Pow(f, 1/2.4) - 0.055
	}
	return uint8(math.Round(clamp01(s) * 255))
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
