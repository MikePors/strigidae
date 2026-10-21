package theme

import (
	"fmt"
	"hash/fnv"
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

// Palette is a Base24 colour scheme: the only place colour values enter the
// theme. Roles (roles.go) are derived from it; styles never read it.
type Palette struct {
	Name    string
	Variant Variant
	Colors  [numSlots]color.Color
}

// Slot returns the colour in slot s. Only roles.go and the palette
// constructors may call it.
func (p Palette) Slot(s Slot) color.Color { return p.Colors[s] }

// ID identifies the palette's contents. Caches of rendered output include it
// so a theme change cannot show stale colours.
func (p Palette) ID() string {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d", p.Variant)
	for _, c := range p.Colors {
		_, _ = h.Write([]byte(HexString(c)))
	}
	return fmt.Sprintf("%s-%016x", p.Name, h.Sum64())
}

// FromBase16 builds a Base24 palette from the 16 Base16 colours (base00 to
// base0F, in order). base10 and base11 fall back to base00, and the bright
// accents base12 to base17 to the matching normal accents, as tinted-theming
// documents for Base16 schemes.
func FromBase16(name string, v Variant, base16 [16]color.Color) Palette {
	p := Palette{Name: name, Variant: v}
	copy(p.Colors[:], base16[:])
	p.Colors[Base10] = base16[Base00]
	p.Colors[Base11] = base16[Base00]
	p.Colors[Base12] = base16[Base08] // red
	p.Colors[Base13] = base16[Base0A] // yellow
	p.Colors[Base14] = base16[Base0B] // green
	p.Colors[Base15] = base16[Base0C] // cyan
	p.Colors[Base16] = base16[Base0D] // blue
	p.Colors[Base17] = base16[Base0E] // magenta
	return p
}

// FromANSI builds a Base24 palette from a terminal's colours: its background,
// foreground and the 16 ANSI colours (black, red, green, yellow, blue,
// magenta, cyan, white, then the bright versions). Normal colours become the
// accents, bright colours base12-base17. ANSI has no intermediate shades or
// orange, so the shades blend the background toward the foreground and orange
// blends red with yellow. The variant follows the background's luminance.
func FromANSI(name string, bg, fg color.Color, ansi [16]color.Color) Palette {
	v := Dark
	if luminance(bg) > 0.5 {
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
	p.Colors[Base00] = bg
	p.Colors[Base01] = blend(bg, fg, 0.12)
	p.Colors[Base02] = blend(bg, fg, 0.25)
	p.Colors[Base03] = blend(bg, fg, 0.45)
	p.Colors[Base04] = blend(bg, fg, 0.7)
	p.Colors[Base05] = fg
	p.Colors[Base06] = blend(fg, far, 0.5)
	p.Colors[Base07] = far
	p.Colors[Base08] = ansi[1]
	p.Colors[Base09] = orange
	p.Colors[Base0A] = ansi[3]
	p.Colors[Base0B] = ansi[2]
	p.Colors[Base0C] = ansi[6]
	p.Colors[Base0D] = ansi[4]
	p.Colors[Base0E] = ansi[5]
	p.Colors[Base0F] = blend(orange, bg, 0.4)
	p.Colors[Base10] = blend(bg, color.Black, 0.2)
	p.Colors[Base11] = blend(bg, color.Black, 0.4)
	p.Colors[Base12] = ansi[9]
	p.Colors[Base13] = ansi[11]
	p.Colors[Base14] = ansi[10]
	p.Colors[Base15] = ansi[14]
	p.Colors[Base16] = ansi[12]
	p.Colors[Base17] = ansi[13]
	return p
}
