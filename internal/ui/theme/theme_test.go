package theme

import (
	"image/color"
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/google/go-cmp/cmp"
)

func TestFromBase16Fallbacks(t *testing.T) {
	var b16 [16]color.Color
	for i := range b16 {
		b16[i] = color.RGBA{R: uint8(i * 10), G: uint8(i), B: 7, A: 255}
	}
	p := FromBase16("x", Light, b16)
	if p.Variant != Light || p.Name != "x" {
		t.Errorf("name/variant = %q/%v", p.Name, p.Variant)
	}
	for i := range b16 {
		if p.Colors[i] != b16[i] {
			t.Errorf("slot %d = %v, want %v", i, p.Colors[i], b16[i])
		}
	}
	tests := []struct {
		slot, from Slot
	}{
		{Base10, Base00},
		{Base11, Base00},
		{Base12, Base08},
		{Base13, Base0A},
		{Base14, Base0B},
		{Base15, Base0C},
		{Base16, Base0D},
		{Base17, Base0E},
	}
	for _, tc := range tests {
		if p.Colors[tc.slot] != b16[tc.from] {
			t.Errorf("slot %d = %v, want slot %d (%v)", tc.slot, p.Colors[tc.slot], tc.from, b16[tc.from])
		}
	}
}

func ansiFixture() (bg, fg color.Color, ansi [16]color.Color) {
	names := []string{
		"#000000", "#cc0000", "#00cc00", "#cccc00", "#0000cc", "#cc00cc", "#00cccc", "#cccccc",
		"#555555", "#ff5555", "#55ff55", "#ffff55", "#5555ff", "#ff55ff", "#55ffff", "#ffffff",
	}
	for i, n := range names {
		ansi[i] = hex(n)
	}
	return hex("#101010"), hex("#e0e0e0"), ansi
}

func TestFromANSI(t *testing.T) {
	bg, fg, ansi := ansiFixture()
	p := FromANSI("term", bg, fg, ansi)
	if p.Variant != Dark {
		t.Errorf("dark background gave variant %v", p.Variant)
	}
	direct := []struct {
		slot Slot
		want color.Color
	}{
		{Base00, bg},
		{Base05, fg},
		{Base08, ansi[1]},
		{Base0B, ansi[2]},
		{Base0A, ansi[3]},
		{Base0D, ansi[4]},
		{Base0E, ansi[5]},
		{Base0C, ansi[6]},
		{Base12, ansi[9]},
		{Base14, ansi[10]},
		{Base13, ansi[11]},
		{Base16, ansi[12]},
		{Base17, ansi[13]},
		{Base15, ansi[14]},
		{Base07, ansi[15]},
	}
	for _, tc := range direct {
		if got := p.Colors[tc.slot]; got != tc.want {
			t.Errorf("slot %d = %s, want %s", tc.slot, HexString(got), HexString(tc.want))
		}
	}
	if got, want := HexString(p.Colors[Base09]), "#cc6600"; got != want {
		t.Errorf("orange = %s, want %s (red blended with yellow)", got, want)
	}
	// The shades climb monotonically from background to foreground.
	prev := luminance(p.Colors[Base00])
	for _, s := range []Slot{Base01, Base02, Base03, Base04, Base05, Base06, Base07} {
		l := luminance(p.Colors[s])
		if l <= prev {
			t.Errorf("slot %d (luminance %.3f) is not brighter than the one before (%.3f)", s, l, prev)
		}
		prev = l
	}
	for s, c := range p.Colors {
		if c == nil {
			t.Errorf("slot %d unset", s)
		}
	}
}

func TestFromANSILightBackground(t *testing.T) {
	_, _, ansi := ansiFixture()
	p := FromANSI("term", hex("#fafafa"), hex("#202020"), ansi)
	if p.Variant != Light {
		t.Errorf("light background gave variant %v", p.Variant)
	}
	if got, want := p.Colors[Base07], ansi[0]; got != want {
		t.Errorf("brightest foreground = %s, want ANSI black %s", HexString(got), HexString(want))
	}
}

func TestBuiltinPalettesComplete(t *testing.T) {
	for _, p := range []Palette{builtinDark, builtinLight} {
		for s, c := range p.Colors {
			if c == nil {
				t.Errorf("%s: slot %d unset", p.Name, s)
			}
		}
	}
	if builtinDark.ID() == builtinLight.ID() {
		t.Error("palettes share an ID")
	}
}

func TestVariants(t *testing.T) {
	if !New(true).IsDark || New(false).IsDark || !Default().IsDark {
		t.Error("IsDark does not follow the palette variant")
	}
	if diff := cmp.Diff(New(true).ID, FromPalette(builtinDark).ID); diff != "" {
		t.Errorf("New(true) is not the dark built-in: %s", diff)
	}
}

// TestBuiltinPalettesPassGuard proves the contrast guard leaves the built-in
// palettes untouched: they were chosen to be readable.
func TestBuiltinPalettesPassGuard(t *testing.T) {
	for _, p := range []Palette{builtinDark, builtinLight} {
		raw := defaultRoles(p)
		guarded := raw
		guarded.guard()
		if diff := cmp.Diff(colorHexes(raw), colorHexes(guarded)); diff != "" {
			t.Errorf("%s: the guard changed roles (-derived +guarded):\n%s", p.Name, diff)
		}
	}
}

func colorHexes(r Roles) map[string]string {
	out := map[string]string{}
	for _, f := range r.fields() {
		out[f.name] = HexString(*f.c)
	}
	return out
}

func TestGuardFixesUnreadableRoles(t *testing.T) {
	bad := builtinDark
	bad.Colors[Base05] = bad.Colors[Base00] // text invisible on the page
	bad.Colors[Base03] = bad.Colors[Base01] // muted invisible on the bar
	bad.Colors[Base0B] = bad.Colors[Base00] // success invisible
	r := deriveRoles(bad, Roles{})
	checks := []struct {
		name string
		fg   color.Color
		bg   color.Color
		min  float64
	}{
		{"text", r.Text, r.Background, minTextContrast},
		{"text on bar", r.Text, r.Surface, minTextContrast},
		{"muted", r.Muted, r.Background, minAccentContrast},
		{"muted on bar", r.Muted, r.Surface, minAccentContrast},
		{"success", r.Success, r.Background, minAccentContrast},
	}
	for _, tc := range checks {
		if got := contrast(tc.fg, tc.bg); got < tc.min {
			t.Errorf("%s contrast = %.2f, want at least %.1f", tc.name, got, tc.min)
		}
	}
}

func TestGuardLightBackgroundGoesDark(t *testing.T) {
	bad := builtinLight
	bad.Colors[Base05] = bad.Colors[Base00]
	r := deriveRoles(bad, Roles{})
	if luminance(r.Text) >= luminance(r.Background) {
		t.Errorf("text %s was not darkened against a light background", HexString(r.Text))
	}
}

func TestOverridesApplyAfterDerivation(t *testing.T) {
	ov := Roles{Added: hex("#00ff00"), HunkTint: hex("#123456")}
	r := deriveRoles(builtinDark, ov)
	if got := HexString(r.Added); got != "#00ff00" && contrast(r.Added, r.Background) >= minAccentContrast {
		t.Errorf("Added = %s, want the override", got)
	}
	if got := HexString(r.HunkTint); got != "#123456" {
		t.Errorf("HunkTint = %s, want the override", got)
	}
	// An unset tint follows the overridden role it blends.
	base := deriveRoles(builtinDark, Roles{})
	if HexString(r.AddedTint) == HexString(base.AddedTint) {
		t.Error("AddedTint ignored the Added override")
	}
	if HexString(r.RemovedTint) != HexString(base.RemovedTint) {
		t.Error("RemovedTint changed without an override")
	}
}

func TestOverridesAreGuarded(t *testing.T) {
	ov := Roles{Text: builtinDark.Colors[Base00]}
	r := deriveRoles(builtinDark, ov)
	if contrast(r.Text, r.Background) < minTextContrast {
		t.Error("an unreadable override escaped the guard")
	}
}

// styleColors collects every colour a lipgloss style sets.
func styleColors(s lipgloss.Style) []string {
	var out []string
	for _, c := range []color.Color{
		s.GetForeground(), s.GetBackground(),
		s.GetBorderTopForeground(),
	} {
		if c == nil {
			out = append(out, "")
			continue
		}
		if _, ok := c.(lipgloss.NoColor); ok {
			out = append(out, "")
			continue
		}
		out = append(out, HexString(c))
	}
	return out
}

// walkStyles calls f with the path and value of every style in v.
func walkStyles(v reflect.Value, path string, f func(path string, s lipgloss.Style)) {
	styleType := reflect.TypeOf(lipgloss.Style{})
	for i := range v.NumField() {
		fv, name := v.Field(i), path+v.Type().Field(i).Name
		switch {
		case fv.Type() == styleType:
			f(name, fv.Interface().(lipgloss.Style))
		case fv.Kind() == reflect.Struct && v.Type().Field(i).IsExported():
			walkStyles(fv, name+".", f)
		}
	}
}

// TestNothingIsHardcoded builds themes from two very different palettes and
// checks every style that has a colour in one has a different colour in the
// other. A style still using a literal colour would be identical in both.
func TestNothingIsHardcoded(t *testing.T) {
	loud := builtinDark
	loud.Name = "loud"
	for i := range loud.Colors {
		loud.Colors[i] = color.RGBA{R: uint8(255 - i*9), G: uint8(40 + i*8), B: uint8(i * 10), A: 255}
	}
	a, b := FromPalette(builtinDark), FromPalette(loud)
	if a.ID == b.ID {
		t.Fatal("setup: palettes share an ID")
	}
	got := map[string][]string{}
	walkStyles(reflect.ValueOf(a), "", func(path string, s lipgloss.Style) { got[path] = styleColors(s) })
	checked := 0
	walkStyles(reflect.ValueOf(b), "", func(path string, s lipgloss.Style) {
		ca, cb := got[path], styleColors(s)
		for i := range ca {
			if ca[i] == "" && cb[i] == "" {
				continue
			}
			checked++
			if ca[i] == cb[i] {
				t.Errorf("%s colour %d is %q in both palettes; is it hardcoded?", path, i, ca[i])
			}
		}
	})
	if checked < 50 {
		t.Errorf("only %d colours compared; the walk is missing styles", checked)
	}
}

func TestChromaStyleFollowsRoles(t *testing.T) {
	th := New(true)
	st := newChromaStyle("t", th.Roles)
	for _, tc := range []struct {
		name string
		tok  chroma.TokenType
		want color.Color
	}{
		{"string", chroma.LiteralString, th.Roles.SynString},
		{"keyword", chroma.Keyword, th.Roles.SynKeyword},
		{"function", chroma.NameFunction, th.Roles.SynFunction},
		{"number", chroma.LiteralNumberInteger, th.Roles.SynNumber},
		{"type", chroma.KeywordType, th.Roles.SynType},
		{"comment", chroma.CommentSingle, th.Roles.SynComment},
	} {
		if got, want := st.Get(tc.tok).Colour.String(), HexString(tc.want); got != want {
			t.Errorf("%s = %s, want %s", tc.name, got, want)
		}
	}
	if New(true).ChromaStyleName() == New(false).ChromaStyleName() {
		t.Error("dark and light share a Chroma style name")
	}
}
