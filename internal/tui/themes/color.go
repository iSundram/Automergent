package themes

import (
	"image/color"
	"math"
)

// Mix blends two colors linearly. t=0 returns a, t=1 returns b.
func Mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	mix := func(x, y uint32) uint8 {
		v := float64(x>>8)*(1-t) + float64(y>>8)*t
		return uint8(v + 0.5)
	}
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 255}
}

// Luminance returns the relative luminance of a color (0..1).
func Luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	fr := srgbToLinear(float64(r>>8) / 255)
	fg := srgbToLinear(float64(g>>8) / 255)
	fb := srgbToLinear(float64(b>>8) / 255)
	return 0.2126*fr + 0.7152*fg + 0.0722*fb
}

// IsDark reports whether a color reads as dark (luminance < 0.4).
func IsDark(c color.Color) bool {
	return Luminance(c) < 0.4
}

func srgbToLinear(x float64) float64 {
	if x <= 0.03928 {
		return x / 12.92
	}
	return math.Pow(x, 2.4)
}
