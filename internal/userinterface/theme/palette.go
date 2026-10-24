package theme

import (
	"fmt"
	"image/color"
)

// Variant says whether a palette is meant for a dark or a light background.
type Variant int

// Palette variants.
const (
	Dark Variant = iota
	Light
)

// Slot names one of the 24 Base24 palette slots.
type Slot int

// The Base24 slots. In a dark palette base00-base07 run from the darkest
// background to the brightest foreground; a light palette runs them the other
// way (base00 is the lightest). base08-base0F are accents, base10-base11
// darker backgrounds and base12-base17 bright accents.
const (
	Base00 Slot = iota // default background
	Base01             // lighter background (status bars)
	Base02             // selection background
	Base03             // comments, muted text
	Base04             // dark foreground
	Base05             // default foreground
	Base06             // light foreground
	Base07             // lightest foreground
	Base08             // red
	Base09             // orange
	Base0A             // yellow
	Base0B             // green
	Base0C             // cyan
	Base0D             // blue
	Base0E             // magenta
	Base0F             // brown
	Base10             // darker background
	Base11             // darkest background
	Base12             // bright red
	Base13             // bright yellow
	Base14             // bright green
	Base15             // bright cyan
	Base16             // bright blue
	Base17             // bright magenta
	numberSlots
)

// String returns the slot's Base24 name, such as "base0B".
func (slot Slot) String() string { return fmt.Sprintf("base%02X", int(slot)) }

// Palette is a Base24 colour scheme: the only place colour values enter the
// theme. Roles (roles.go) are derived from it; styles never read it. The slots
// are unexported so only this package can index them.
type Palette struct {
	Name    string
	Variant Variant
	colors  [numberSlots]color.Color
}

// Slot returns the colour in the given slot. Only roles.go and the palette
// constructors may call it.
func (palette Palette) Slot(slot Slot) color.Color { return palette.colors[slot] }

// check reports the first unset slot.
func (palette Palette) check() error {
	for index, colour := range palette.colors {
		if colour == nil {
			return fmt.Errorf("palette %q: missing slot %s", palette.Name, Slot(index))
		}
	}
	return nil
}

// FromBase16 builds a Base24 palette from the 16 Base16 colours (base00 to
// base0F, in order). base10 and base11 fall back to base00, and the bright
// accents base12 to base17 to the matching normal accents, as tinted-theming
// documents for Base16 schemes. It returns an error naming the first missing
// (nil) slot.
func FromBase16(name string, variant Variant, base16 [16]color.Color) (Palette, error) {
	palette := Palette{Name: name, Variant: variant}
	copy(palette.colors[:], base16[:])
	palette.colors[Base10] = base16[Base00]
	palette.colors[Base11] = base16[Base00]
	palette.colors[Base12] = base16[Base08] // red
	palette.colors[Base13] = base16[Base0A] // yellow
	palette.colors[Base14] = base16[Base0B] // green
	palette.colors[Base15] = base16[Base0C] // cyan
	palette.colors[Base16] = base16[Base0D] // blue
	palette.colors[Base17] = base16[Base0E] // magenta
	for index := range base16 {
		if base16[index] == nil {
			return Palette{}, fmt.Errorf("palette %q: missing slot %s", name, Slot(index))
		}
	}
	return palette, nil
}

// FromBase24 builds a palette from all 24 Base24 colours (base00 to base17, in
// order). It returns an error naming the first missing (nil) slot.
func FromBase24(name string, variant Variant, base24 [24]color.Color) (Palette, error) {
	palette := Palette{Name: name, Variant: variant, colors: base24}
	if err := palette.check(); err != nil {
		return Palette{}, err
	}
	return palette, nil
}

// VariantOf says which variant a palette with this background colour (base00)
// is: Dark when white text reads better on it than black.
func VariantOf(background color.Color) Variant {
	if extreme(background) == color.Color(color.Black) {
		return Light
	}
	return Dark
}

// minimumDistinctContrast is how far a colour must be from the background and the
// foreground (as a WCAG contrast ratio) to count as a distinct shade.
const minimumDistinctContrast = 1.1

// FromANSI builds a Base24 palette from a terminal's colours: its background,
// foreground and the 16 ANSI colours (black, red, green, yellow, blue,
// magenta, cyan, white, then the bright versions). Normal colours become the
// accents, bright colours base12-base17, and bright black (the colour most
// terminal themes use for comments) base03, unless it is the same as the
// background or foreground, as in Solarized, where it is blended like the
// other shades. ANSI has no intermediate shades or orange, so the shades blend
// the background toward the foreground and orange blends red with yellow. The variant follows whether black or
// white reads better on the background. It returns an error naming a missing
// (nil) input.
func FromANSI(name string, background, foreground color.Color, ansi [16]color.Color) (Palette, error) {
	if background == nil || foreground == nil {
		return Palette{}, fmt.Errorf("palette %q: missing background or foreground", name)
	}
	for i, colour := range ansi {
		if colour == nil {
			return Palette{}, fmt.Errorf("palette %q: missing ANSI colour %d", name, i)
		}
	}
	variant := VariantOf(background)
	// The ANSI black or white furthest from the background supplies the
	// brightest foreground (base07).
	colour := ansi[15]
	if contrast(ansi[0], background) > contrast(ansi[15], background) {
		colour = ansi[0]
	}
	orange := blend(ansi[1], ansi[3], 0.5)

	palette := Palette{Name: name, Variant: variant}
	palette.colors[Base00] = background
	palette.colors[Base01] = blend(background, foreground, 0.12)
	palette.colors[Base02] = blend(background, foreground, 0.25)
	palette.colors[Base03] = blend(background, foreground, 0.45)
	if contrast(ansi[8], background) >= minimumDistinctContrast && contrast(ansi[8], foreground) >= minimumDistinctContrast {
		palette.colors[Base03] = ansi[8]
	}
	palette.colors[Base04] = blend(background, foreground, 0.7)
	palette.colors[Base05] = foreground
	palette.colors[Base06] = blend(foreground, colour, 0.5)
	palette.colors[Base07] = colour
	palette.colors[Base08] = ansi[1]
	palette.colors[Base09] = orange
	palette.colors[Base0A] = ansi[3]
	palette.colors[Base0B] = ansi[2]
	palette.colors[Base0C] = ansi[6]
	palette.colors[Base0D] = ansi[4]
	palette.colors[Base0E] = ansi[5]
	palette.colors[Base0F] = blend(orange, background, 0.4)
	palette.colors[Base10] = blend(background, color.Black, 0.2)
	palette.colors[Base11] = blend(background, color.Black, 0.4)
	palette.colors[Base12] = ansi[9]
	palette.colors[Base13] = ansi[11]
	palette.colors[Base14] = ansi[10]
	palette.colors[Base15] = ansi[14]
	palette.colors[Base16] = ansi[12]
	palette.colors[Base17] = ansi[13]
	return palette, nil
}
