package theme

import (
	"fmt"
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
)

// hex parses a "#rrggbb" string. It is how palette definitions spell colours.
func hex(s string) color.Color { return lipgloss.Color(s) }

// HexString formats c as "#rrggbb" for renderers that take colour strings
// (Glamour, Chroma).
func HexString(c color.Color) string {
	r, g, b := rgb8(c)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func rgb8(c color.Color) (r, g, b uint8) {
	r16, g16, b16, _ := c.RGBA()
	return uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8)
}

func newRGB(r, g, b float64) color.Color {
	clamp := func(v float64) uint8 { return uint8(math.Round(math.Min(255, math.Max(0, v)))) }
	return color.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 255}
}

// blend mixes a toward b: t=0 is a, t=1 is b. It mixes in sRGB, which is
// plain and predictable for the small ratios tints use.
func blend(a, b color.Color, t float64) color.Color {
	ar, ag, ab := rgb8(a)
	br, bg, bb := rgb8(b)
	mix := func(x, y uint8) float64 { return float64(x)*(1-t) + float64(y)*t }
	return newRGB(mix(ar, br), mix(ag, bg), mix(ab, bb))
}

// luminance is the WCAG relative luminance of c.
func luminance(c color.Color) float64 {
	r, g, b := rgb8(c)
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrast is the WCAG contrast ratio of a and b, from 1 (identical) to 21.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// nudge moves fg away from bg (towards black on a light bg, white on a dark
// one) in small steps until it reaches the contrast ratio min, or runs out of
// room. A colour that already passes is returned unchanged.
func nudge(fg, bg color.Color, minRatio float64) color.Color {
	if contrast(fg, bg) >= minRatio {
		return fg
	}
	var target color.Color = color.White
	if luminance(bg) > 0.5 {
		target = color.Black
	}
	out := fg
	for step := 1; step <= 20; step++ {
		out = blend(fg, target, float64(step)*0.05)
		if contrast(out, bg) >= minRatio {
			break
		}
	}
	return out
}
