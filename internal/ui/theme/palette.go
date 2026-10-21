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
	numSlots
)

// String returns the slot's Base24 name, such as "base0B".
func (s Slot) String() string { return fmt.Sprintf("base%02X", int(s)) }

// Palette is a Base24 colour scheme: the only place colour values enter the
// theme. Roles (roles.go) are derived from it; styles never read it. The slots
// are unexported so only this package can index them.
type Palette struct {
	Name    string
	Variant Variant
	colors  [numSlots]color.Color
}

// Slot returns the colour in slot s. Only roles.go and the palette
// constructors may call it.
func (p Palette) Slot(s Slot) color.Color { return p.colors[s] }

// check reports the first unset slot.
func (p Palette) check() error {
	for s, c := range p.colors {
		if c == nil {
			return fmt.Errorf("palette %q: missing slot %s", p.Name, Slot(s))
		}
	}
	return nil
}

// FromBase16 builds a Base24 palette from the 16 Base16 colours (base00 to
// base0F, in order). base10 and base11 fall back to base00, and the bright
// accents base12 to base17 to the matching normal accents, as tinted-theming
// documents for Base16 schemes. It returns an error naming the first missing
// (nil) slot.
func FromBase16(name string, v Variant, base16 [16]color.Color) (Palette, error) {
	p := Palette{Name: name, Variant: v}
	copy(p.colors[:], base16[:])
	p.colors[Base10] = base16[Base00]
	p.colors[Base11] = base16[Base00]
	p.colors[Base12] = base16[Base08] // red
	p.colors[Base13] = base16[Base0A] // yellow
	p.colors[Base14] = base16[Base0B] // green
	p.colors[Base15] = base16[Base0C] // cyan
	p.colors[Base16] = base16[Base0D] // blue
	p.colors[Base17] = base16[Base0E] // magenta
	for s := range base16 {
		if base16[s] == nil {
			return Palette{}, fmt.Errorf("palette %q: missing slot %s", name, Slot(s))
		}
	}
	return p, nil
}

// FromANSI builds a Base24 palette from a terminal's colours: its background,
// foreground and the 16 ANSI colours (black, red, green, yellow, blue,
// magenta, cyan, white, then the bright versions). Normal colours become the
// accents, bright colours base12-base17, and bright black (the colour most
// terminal themes use for comments) base03. ANSI has no other intermediate
// shades or orange, so the shades blend the background toward the foreground
// and orange blends red with yellow. The variant follows whether black or
// white reads better on the background. It returns an error naming a missing
// (nil) input.
func FromANSI(name string, bg, fg color.Color, ansi [16]color.Color) (Palette, error) {
	if bg == nil || fg == nil {
		return Palette{}, fmt.Errorf("palette %q: missing background or foreground", name)
	}
	for i, c := range ansi {
		if c == nil {
			return Palette{}, fmt.Errorf("palette %q: missing ANSI colour %d", name, i)
		}
	}
	v := Dark
	if extreme(bg) == color.Color(color.Black) {
		v = Light
	}
	// The ANSI black or white furthest from the background supplies the
	// brightest foreground (base07).
	far := ansi[15]
	if contrast(ansi[0], bg) > contrast(ansi[15], bg) {
		far = ansi[0]
	}
	orange := blend(ansi[1], ansi[3], 0.5)

	p := Palette{Name: name, Variant: v}
	p.colors[Base00] = bg
	p.colors[Base01] = blend(bg, fg, 0.12)
	p.colors[Base02] = blend(bg, fg, 0.25)
	p.colors[Base03] = ansi[8]
	p.colors[Base04] = blend(bg, fg, 0.7)
	p.colors[Base05] = fg
	p.colors[Base06] = blend(fg, far, 0.5)
	p.colors[Base07] = far
	p.colors[Base08] = ansi[1]
	p.colors[Base09] = orange
	p.colors[Base0A] = ansi[3]
	p.colors[Base0B] = ansi[2]
	p.colors[Base0C] = ansi[6]
	p.colors[Base0D] = ansi[4]
	p.colors[Base0E] = ansi[5]
	p.colors[Base0F] = blend(orange, bg, 0.4)
	p.colors[Base10] = blend(bg, color.Black, 0.2)
	p.colors[Base11] = blend(bg, color.Black, 0.4)
	p.colors[Base12] = ansi[9]
	p.colors[Base13] = ansi[11]
	p.colors[Base14] = ansi[10]
	p.colors[Base15] = ansi[14]
	p.colors[Base16] = ansi[12]
	p.colors[Base17] = ansi[13]
	return p, nil
}
