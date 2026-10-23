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
	parsed, err := ParseHex(text)
	if err != nil {
		panic(err)
	}
	return parsed
}

// HexString formats c as "#rrggbb" for renderers that take colour strings
// (Glamour, Chroma).
func HexString(colour color.Color) string {
	red, green, blue := rgb8(colour)
	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}

func rgb8(colour color.Color) (red, green, blue uint8) {
	red16, green16, blue16, _ := colour.RGBA()
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
	mix := func(from, to uint8) float64 { return float64(from)*(1-fraction) + float64(to)*fraction }
	return newRGB(mix(firstRed, secondRed), mix(firstGreen, secondGreen), mix(firstBlue, secondBlue))
}

// luminance is the WCAG relative luminance of c.
func luminance(colour color.Color) float64 {
	red, green, blue := rgb8(colour)
	linearize := func(channel uint8) float64 {
		normalised := float64(channel) / 255
		if normalised <= 0.03928 {
			return normalised / 12.92
		}
		return math.Pow((normalised+0.055)/1.055, 2.4)
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

func toOKLCH(colour color.Color) oklch {
	red8, green8, blue8 := rgb8(colour)
	linearize := func(channel uint8) float64 {
		normalised := float64(channel) / 255
		if normalised <= 0.04045 {
			return normalised / 12.92
		}
		return math.Pow((normalised+0.055)/1.055, 2.4)
	}
	red, green, blue := linearize(red8), linearize(green8), linearize(blue8)
	long := math.Cbrt(0.4122214708*red + 0.5363325363*green + 0.0514459929*blue)
	medium := math.Cbrt(0.2119034982*red + 0.6806995451*green + 0.1073969566*blue)
	short := math.Cbrt(0.0883024619*red + 0.2817188376*green + 0.6299787005*blue)
	lab := [3]float64{
		0.2104542553*long + 0.7936177850*medium - 0.0040720468*short,
		1.9779984951*long - 2.4285922050*medium + 0.4505937099*short,
		0.0259040371*long + 0.7827717662*medium - 0.8086757660*short,
	}
	return oklch{lab[0], math.Hypot(lab[1], lab[2]), math.Atan2(lab[2], lab[1])}
}

func (value oklch) color() color.Color {
	axisA, axisB := value.chroma*math.Cos(value.hue), value.chroma*math.Sin(value.hue)
	cube := func(number float64) float64 { return number * number * number }
	long := cube(value.lightness + 0.3963377774*axisA + 0.2158037573*axisB)
	medium := cube(value.lightness - 0.1055613458*axisA - 0.0638541728*axisB)
	short := cube(value.lightness - 0.0894841775*axisA - 1.2914855480*axisB)
	encode := func(linear float64) float64 {
		linear = math.Min(1, math.Max(0, linear))
		if linear <= 0.0031308 {
			return 255 * 12.92 * linear
		}
		return 255 * (1.055*math.Pow(linear, 1/2.4) - 0.055)
	}
	return newRGB(
		encode(4.0767416621*long-3.3077115913*medium+0.2309699292*short),
		encode(-1.2684380046*long+2.6097574011*medium-0.3413193965*short),
		encode(-0.0041960863*long-0.7034186147*medium+1.7076147010*short),
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
