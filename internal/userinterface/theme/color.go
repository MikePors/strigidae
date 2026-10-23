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
func ParseHex(text string) (color.Color, error) {
	if !hexColour.MatchString(text) {
		return nil, fmt.Errorf("invalid colour %q: want #rrggbb", text)
	}
	return lipgloss.Color(text), nil
}

// hex is ParseHex for the built-in palettes: a malformed literal is a
// programming error and panics when the palette is constructed.
func hex(text string) color.Color {
	color, err := ParseHex(text)
	if err != nil {
		panic(err)
	}
	return color
}

// HexString formats c as "#rrggbb" for renderers that take colour strings
// (Glamour, Chroma).
func HexString(color color.Color) string {
	red, green, blue := rgb8(color)
	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}

func rgb8(color color.Color) (red, green, blue uint8) {
	red16, green16, blue16, _ := color.RGBA()
	return uint8(red16 >> 8), uint8(green16 >> 8), uint8(blue16 >> 8)
}

func newRGB(red, green, blue float64) color.Color {
	clamp := func(value float64) uint8 { return uint8(math.Round(math.Min(255, math.Max(0, value)))) }
	return color.RGBA{R: clamp(red), G: clamp(green), B: clamp(blue), A: 255}
}

// blend mixes a toward b: t=0 is a, t=1 is b. It mixes in sRGB, which is
// plain and predictable for the small ratios shades use.
func blend(first, second color.Color, fraction float64) color.Color {
	firstRed, firstGreen, firstBlue := rgb8(first)
	secondRed, secondGreen, secondBlue := rgb8(second)
	mix := func(column, row uint8) float64 { return float64(column)*(1-fraction) + float64(row)*fraction }
	return newRGB(mix(firstRed, secondRed), mix(firstGreen, secondGreen), mix(firstBlue, secondBlue))
}

// luminance is the WCAG relative luminance of c.
func luminance(color color.Color) float64 {
	red, green, blue := rgb8(color)
	linearize := func(value uint8) float64 {
		saturation := float64(value) / 255
		if saturation <= 0.03928 {
			return saturation / 12.92
		}
		return math.Pow((saturation+0.055)/1.055, 2.4)
	}
	return 0.2126*linearize(red) + 0.7152*linearize(green) + 0.0722*linearize(blue)
}

// contrast is the WCAG contrast ratio of a and b, from 1 (identical) to 21.
func contrast(first, second color.Color) float64 {
	firstLuminance, secondLuminance := luminance(first), luminance(second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + 0.05) / (secondLuminance + 0.05)
}

// extreme returns black or white, whichever reads better on bg. The two tie
// at a luminance of about 0.18, not 0.5.
func extreme(background color.Color) color.Color {
	if contrast(color.Black, background) > contrast(color.White, background) {
		return color.Black
	}
	return color.White
}

// nudge moves fg away from bg (towards black or white, whichever contrasts
// more with bg) in small steps until it reaches the contrast ratio min, or
// runs out of room. A colour that already passes is returned unchanged.
func nudge(foreground, background color.Color, minimumRatio float64) color.Color {
	if contrast(foreground, background) >= minimumRatio {
		return foreground
	}
	target := extreme(background)
	out := foreground
	for step := 1; step <= 20; step++ {
		out = blend(foreground, target, float64(step)*0.05)
		if contrast(out, background) >= minimumRatio {
			break
		}
	}
	return out
}

// oklch is a colour in the OKLab polar form: perceptual lightness, chroma
// and hue. Mixing there keeps a tint's hue and saturation where an sRGB blend
// would grey them.
type oklch struct{ lightness, chroma, hue float64 }

func toOKLCH(column color.Color) oklch {
	red8, green8, blue8 := rgb8(column)
	linearize := func(value uint8) float64 {
		saturation := float64(value) / 255
		if saturation <= 0.04045 {
			return saturation / 12.92
		}
		return math.Pow((saturation+0.055)/1.055, 2.4)
	}
	red, green, blue := linearize(red8), linearize(green8), linearize(blue8)
	lightness := math.Cbrt(0.4122214708*red + 0.5363325363*green + 0.0514459929*blue)
	value := math.Cbrt(0.2119034982*red + 0.6806995451*green + 0.1073969566*blue)
	saturation := math.Cbrt(0.0883024619*red + 0.2817188376*green + 0.6299787005*blue)
	lab := [3]float64{
		0.2104542553*lightness + 0.7936177850*value - 0.0040720468*saturation,
		1.9779984951*lightness - 2.4285922050*value + 0.4505937099*saturation,
		0.0259040371*lightness + 0.7827717662*value - 0.8086757660*saturation,
	}
	return oklch{lab[0], math.Hypot(lab[1], lab[2]), math.Atan2(lab[2], lab[1])}
}

func (color oklch) color() color.Color {
	first, blue := color.chroma*math.Cos(color.hue), color.chroma*math.Sin(color.hue)
	cube := func(value float64) float64 { return value * value * value }
	lightness := cube(color.lightness + 0.3963377774*first + 0.2158037573*blue)
	value := cube(color.lightness - 0.1055613458*first - 0.0638541728*blue)
	saturation := cube(color.lightness - 0.0894841775*first - 1.2914855480*blue)
	encode := func(value float64) float64 {
		value = math.Min(1, math.Max(0, value))
		if value <= 0.0031308 {
			return 255 * 12.92 * value
		}
		return 255 * (1.055*math.Pow(value, 1/2.4) - 0.055)
	}
	return newRGB(
		encode(4.0767416621*lightness-3.3077115913*value+0.2309699292*saturation),
		encode(-1.2684380046*lightness+2.6097574011*value-0.3413193965*saturation),
		encode(-0.0041960863*lightness-0.7034186147*value+1.7076147010*saturation),
	)
}

// tint makes a row-background colour from a role. The model is OKLCH: the
// result's lightness is the base's plus lightMix of the way to the role's
// (base L + lightMix*dL), its chroma is chromaScale times the role's, and its
// hue is the role's, so a green tint stays green on any base and keeps its
// saturation, which an sRGB blend would grey out.
func tint(base, role color.Color, lightMix, chromaScale float64) color.Color {
	baseOklch, roleOklch := toOKLCH(base), toOKLCH(role)
	return oklch{lightness: baseOklch.lightness + (roleOklch.lightness-baseOklch.lightness)*lightMix, chroma: roleOklch.chroma * chromaScale, hue: roleOklch.hue}.color()
}
