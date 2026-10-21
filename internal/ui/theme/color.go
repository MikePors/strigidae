package theme

import (
	"fmt"
	"image/color"
	"math"
	"regexp"

	"charm.land/lipgloss/v2"
)

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ParseHex parses a "#rrggbb" string. Anything else is an error, so a
// malformed theme value is reported instead of silently drawing as black.
func ParseHex(s string) (color.Color, error) {
	if !hexColour.MatchString(s) {
		return nil, fmt.Errorf("invalid colour %q: want #rrggbb", s)
	}
	return lipgloss.Color(s), nil
}

// hex is ParseHex for the built-in palettes: a malformed literal is a
// programming error and panics when the palette is constructed.
func hex(s string) color.Color {
	c, err := ParseHex(s)
	if err != nil {
		panic(err)
	}
	return c
}

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
// plain and predictable for the small ratios shades use.
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

// extreme returns black or white, whichever reads better on bg. The two tie
// at a luminance of about 0.18, not 0.5.
func extreme(bg color.Color) color.Color {
	if contrast(color.Black, bg) > contrast(color.White, bg) {
		return color.Black
	}
	return color.White
}

// nudge moves fg away from bg (towards black or white, whichever contrasts
// more with bg) in small steps until it reaches the contrast ratio min, or
// runs out of room. A colour that already passes is returned unchanged.
func nudge(fg, bg color.Color, minRatio float64) color.Color {
	if contrast(fg, bg) >= minRatio {
		return fg
	}
	target := extreme(bg)
	out := fg
	for step := 1; step <= 20; step++ {
		out = blend(fg, target, float64(step)*0.05)
		if contrast(out, bg) >= minRatio {
			break
		}
	}
	return out
}

// oklch is a colour in the OKLab polar form: perceptual lightness, chroma
// and hue. Mixing there keeps a tint's hue and saturation where an sRGB blend
// would grey them.
type oklch struct{ l, c, h float64 }

func toOKLCH(col color.Color) oklch {
	r8, g8, b8 := rgb8(col)
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	r, g, b := lin(r8), lin(g8), lin(b8)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	lab := [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
	return oklch{lab[0], math.Hypot(lab[1], lab[2]), math.Atan2(lab[2], lab[1])}
}

func (o oklch) color() color.Color {
	a, b := o.c*math.Cos(o.h), o.c*math.Sin(o.h)
	cube := func(v float64) float64 { return v * v * v }
	l := cube(o.l + 0.3963377774*a + 0.2158037573*b)
	m := cube(o.l - 0.1055613458*a - 0.0638541728*b)
	s := cube(o.l - 0.0894841775*a - 1.2914855480*b)
	enc := func(v float64) float64 {
		v = math.Min(1, math.Max(0, v))
		if v <= 0.0031308 {
			return 255 * 12.92 * v
		}
		return 255 * (1.055*math.Pow(v, 1/2.4) - 0.055)
	}
	return newRGB(
		enc(4.0767416621*l-3.3077115913*m+0.2309699292*s),
		enc(-1.2684380046*l+2.6097574011*m-0.3413193965*s),
		enc(-0.0041960863*l-0.7034186147*m+1.7076147010*s),
	)
}

// tint makes a row-background colour from a role: the base's lightness moves
// toward the role's by lightMix, and the result takes the role's hue with
// chromaScale times its chroma, so a green tint stays green on any base.
func tint(base, role color.Color, lightMix, chromaScale float64) color.Color {
	b, r := toOKLCH(base), toOKLCH(role)
	return oklch{l: b.l + (r.l-b.l)*lightMix, c: r.c * chromaScale, h: r.h}.color()
}
