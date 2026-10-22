package theme

import (
	"bytes"
	"image/color"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/google/go-cmp/cmp"
)

func mustPalette(t *testing.T) func(Palette, error) Palette {
	return func(p Palette, err error) Palette {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
}

func TestFromBase16Fallbacks(t *testing.T) {
	var b16 [16]color.Color
	for i := range b16 {
		b16[i] = color.RGBA{R: uint8(i * 10), G: uint8(i), B: 7, A: 255}
	}
	p := mustPalette(t)(FromBase16("x", Light, b16))
	if p.Variant != Light || p.Name != "x" {
		t.Errorf("name/variant = %q/%v", p.Name, p.Variant)
	}
	for i := range b16 {
		if p.colors[i] != b16[i] {
			t.Errorf("slot %d = %v, want %v", i, p.colors[i], b16[i])
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
		if p.colors[tc.slot] != b16[tc.from] {
			t.Errorf("slot %d = %v, want slot %d (%v)", tc.slot, p.colors[tc.slot], tc.from, b16[tc.from])
		}
	}
}

func TestFromBase16MissingSlot(t *testing.T) {
	var b16 [16]color.Color
	for i := range b16 {
		b16[i] = color.Black
	}
	b16[Base0B] = nil
	_, err := FromBase16("x", Dark, b16)
	if err == nil || !strings.Contains(err.Error(), "base0B") {
		t.Errorf("err = %v, want one naming base0B", err)
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
	p := mustPalette(t)(FromANSI("term", bg, fg, ansi))
	if p.Variant != Dark {
		t.Errorf("dark background gave variant %v", p.Variant)
	}
	direct := []struct {
		slot Slot
		want color.Color
	}{
		{Base00, bg},
		{Base03, ansi[8]}, // bright black: the comment colour terminals pick
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
		if got := p.colors[tc.slot]; got != tc.want {
			t.Errorf("slot %s = %s, want %s", tc.slot, HexString(got), HexString(tc.want))
		}
	}
	if got, want := HexString(p.colors[Base09]), "#cc6600"; got != want {
		t.Errorf("orange = %s, want %s (red blended with yellow)", got, want)
	}
	// The blended shades sit between background and foreground, in order.
	prev := luminance(p.colors[Base00])
	for _, s := range []Slot{Base01, Base02, Base04, Base05, Base06, Base07} {
		l := luminance(p.colors[s])
		if l <= prev {
			t.Errorf("slot %s (luminance %.3f) is not brighter than the one before (%.3f)", s, l, prev)
		}
		prev = l
	}
	if err := p.check(); err != nil {
		t.Error(err)
	}
}

func TestFromANSIMutedShade(t *testing.T) {
	_, _, ansi := ansiFixture()
	blended := func(bg, fg color.Color) string { return HexString(blend(bg, fg, 0.45)) }
	tests := []struct {
		name      string
		bg, fg    string
		bright    string
		wantBlend bool
	}{
		{"distinct bright black is used", "#101010", "#e0e0e0", "#555555", false},
		// Solarized Dark: color8 is the background.
		{"bright black equals the background", "#002b36", "#839496", "#002b36", true},
		{"bright black equals the foreground", "#101010", "#e0e0e0", "#e0e0e0", true},
		{"bright black almost the background", "#101010", "#e0e0e0", "#121212", true},
	}
	for _, tc := range tests {
		a := ansi
		a[8] = hex(tc.bright)
		p := mustPalette(t)(FromANSI("term", hex(tc.bg), hex(tc.fg), a))
		want := tc.bright
		if tc.wantBlend {
			want = blended(hex(tc.bg), hex(tc.fg))
		}
		if got := HexString(p.colors[Base03]); got != want {
			t.Errorf("%s: base03 = %s, want %s", tc.name, got, want)
		}
	}
}

func TestFromANSIVariant(t *testing.T) {
	_, _, ansi := ansiFixture()
	tests := []struct {
		name  string
		bg    string
		want  Variant
		far07 color.Color
	}{
		{"white", "#fafafa", Light, ansi[0]},
		{"black", "#101010", Dark, ansi[15]},
		// Black reads better than white on mid grey; a 0.5 luminance cut-off
		// would call this dark.
		{"mid grey", "#9a9a9a", Light, ansi[0]},
	}
	for _, tc := range tests {
		p := mustPalette(t)(FromANSI("term", hex(tc.bg), hex("#202020"), ansi))
		if p.Variant != tc.want {
			t.Errorf("%s: variant = %v, want %v", tc.name, p.Variant, tc.want)
		}
		if p.colors[Base07] != tc.far07 {
			t.Errorf("%s: base07 = %s, want %s", tc.name, HexString(p.colors[Base07]), HexString(tc.far07))
		}
	}
}

func TestFromANSIMissingInput(t *testing.T) {
	bg, fg, ansi := ansiFixture()
	if _, err := FromANSI("t", nil, fg, ansi); err == nil {
		t.Error("nil background accepted")
	}
	if _, err := FromANSI("t", bg, nil, ansi); err == nil {
		t.Error("nil foreground accepted")
	}
	ansi[11] = nil
	_, err := FromANSI("t", bg, fg, ansi)
	if err == nil || !strings.Contains(err.Error(), "11") {
		t.Errorf("err = %v, want one naming ANSI colour 11", err)
	}
}

func TestParseHex(t *testing.T) {
	for _, s := range []string{"#1a2b3c", "#FFFFFF"} {
		if _, err := ParseHex(s); err != nil {
			t.Errorf("ParseHex(%q) = %v", s, err)
		}
	}
	for _, s := range []string{"", "red", "#fff", "#12345", "#1234567", "#gggggg", "1a2b3c", " #1a2b3c"} {
		if _, err := ParseHex(s); err == nil {
			t.Errorf("ParseHex(%q) succeeded", s)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("hex of a malformed literal did not panic")
		}
	}()
	hex("nope")
}

func TestBuiltinPalettesComplete(t *testing.T) {
	for _, p := range []Palette{builtinDark, builtinLight} {
		if err := p.check(); err != nil {
			t.Error(err)
		}
	}
	if New(true).ID == New(false).ID {
		t.Error("dark and light share an ID")
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
		guarded.guard(nil)
		if diff := cmp.Diff(colorHexes(raw), colorHexes(guarded)); diff != "" {
			t.Errorf("%s: the guard changed roles (-derived +guarded):\n%s", p.Name, diff)
		}
	}
}

// TestBuiltinPalettesGuardQuietly pins that the built-in themes trigger no
// contrast-guard warnings.
func TestBuiltinPalettesGuardQuietly(t *testing.T) {
	buf := captureLog(t)
	New(true)
	New(false)
	if buf.Len() != 0 {
		t.Errorf("built-in themes logged: %s", buf.String())
	}
}

func colorHexes(r Roles) map[string]string {
	out := map[string]string{}
	for _, f := range r.fields() {
		out[f.name] = HexString(*f.c)
	}
	return out
}

// deltaEOK is the Euclidean distance between two colours in OKLab.
func deltaEOK(a, b color.Color) float64 {
	oa, ob := toOKLCH(a), toOKLCH(b)
	da := oa.c*math.Cos(oa.h) - ob.c*math.Cos(ob.h)
	db := oa.c*math.Sin(oa.h) - ob.c*math.Sin(ob.h)
	return math.Sqrt((oa.l-ob.l)*(oa.l-ob.l) + da*da + db*db)
}

// TestBuiltinTintsMatchTheEarlierLook checks the derived diff tints stay
// within a small perceptual distance (OKLab delta E) of the hand-picked
// colours they replaced.
func TestBuiltinTintsMatchTheEarlierLook(t *testing.T) {
	const maxDeltaE = 0.02
	old := map[string]map[string]string{
		"strig-dark": {
			"added-tint": "#12261e", "added-gutter-tint": "#17382a",
			"removed-tint": "#2d1517", "removed-gutter-tint": "#42191d",
			"hunk-tint": "#1b2a3a", "hunk-cursor-tint": "#2c4560",
		},
		"strig-light": {
			"added-tint": "#e6ffec", "added-gutter-tint": "#ccffd8",
			"removed-tint": "#ffebe9", "removed-gutter-tint": "#ffd7d5",
			"hunk-tint": "#ddf4ff", "hunk-cursor-tint": "#c5e4f5",
		},
	}
	n := 0
	for _, p := range []Palette{builtinDark, builtinLight} {
		r := deriveRoles(p, Roles{})
		for _, f := range r.fields() {
			want, ok := old[p.Name][f.name]
			if !ok {
				continue
			}
			n++
			d := deltaEOK(*f.c, hex(want))
			t.Logf("%s %-20s %s vs %s: dE_OK = %.4f", p.Name, f.name, HexString(*f.c), want, d)
			if d > maxDeltaE {
				t.Errorf("%s %s = %s, dE_OK %.4f from %s, want at most %.2f", p.Name, f.name, HexString(*f.c), d, want, maxDeltaE)
			}
		}
	}
	if n != 12 {
		t.Errorf("compared %d tints, want 12", n)
	}
}

func TestGuardFixesUnreadableDerivedRoles(t *testing.T) {
	bad := builtinDark
	bad.colors[Base05] = bad.colors[Base00] // text invisible on the page
	bad.colors[Base03] = bad.colors[Base01] // muted invisible on the bar
	bad.colors[Base0B] = bad.colors[Base00] // success invisible
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
	bad.colors[Base05] = bad.colors[Base00]
	r := deriveRoles(bad, Roles{})
	if luminance(r.Text) >= luminance(r.Background) {
		t.Errorf("text %s was not darkened against a light background", HexString(r.Text))
	}
}

func TestNudgePicksTheBetterExtreme(t *testing.T) {
	// On mid grey, black has far more contrast than white, although the
	// background's luminance is under 0.5.
	bg := hex("#a0a0a0")
	if got := contrast(color.Black, bg); got < 4.5 {
		t.Fatalf("setup: black on %s = %.2f", HexString(bg), got)
	}
	out := nudge(hex("#909090"), bg, 4.5)
	if got := contrast(out, bg); got < 4.5 {
		t.Errorf("nudged to %s with contrast %.2f, want 4.5", HexString(out), got)
	}
	if luminance(out) >= luminance(bg) {
		t.Errorf("nudged toward white (%s)", HexString(out))
	}
}

// captureLog sends slog's default logger to a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestOverridesApplyAfterDerivation(t *testing.T) {
	ov := Roles{Added: hex("#00ff00"), HunkTint: hex("#123456")}
	r := deriveRoles(builtinDark, ov)
	if got := HexString(r.Added); got != "#00ff00" {
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

// TestOverridesAreExact: the guard never changes an override, it only logs.
// The values are Tokyo Night's background and muted text, which sit at 2.76:1.
func TestOverridesAreExact(t *testing.T) {
	buf := captureLog(t)
	ov := Roles{Background: hex("#1a1b26"), Muted: hex("#565f89")}
	r := deriveRoles(builtinDark, ov)
	if got := HexString(r.Muted); got != "#565f89" {
		t.Errorf("Muted = %s, want the exact override #565f89", got)
	}
	log := buf.String()
	if strings.Count(log, "level=WARN") != 1 || !strings.Contains(log, "role=muted") || !strings.Contains(log, "contrast=") {
		t.Errorf("want one warning naming the muted role and its ratio, got:\n%s", log)
	}

	// The same value, derived rather than overridden, is nudged and not logged.
	bad := builtinDark
	bad.colors[Base03] = hex("#565f89")
	bad.colors[Base00] = hex("#1a1b26")
	buf.Reset()
	d := deriveRoles(bad, Roles{})
	if HexString(d.Muted) == "#565f89" {
		t.Error("a derived muted role below the threshold was not nudged")
	}
	if buf.Len() != 0 {
		t.Errorf("derived roles logged: %s", buf.String())
	}
}

func TestOverrideChangesIdentity(t *testing.T) {
	base := FromPalette(builtinDark)
	over := FromPaletteWithOverrides(builtinDark, Roles{SynString: hex("#ff00ff")})
	if base.ID == over.ID {
		t.Fatal("a role override did not change Theme.ID")
	}
	if base.ChromaStyleName() == over.ChromaStyleName() {
		t.Fatal("a role override did not change the Chroma style name")
	}
	got := newChromaStyle(over.ChromaStyleName(), over.Roles).Get(chroma.LiteralString).Colour.String()
	if got != "#ff00ff" {
		t.Errorf("string colour in the overridden Chroma style = %s, want #ff00ff", got)
	}
	if FromPalette(builtinDark).ID != base.ID {
		t.Error("ID is not stable for the same inputs")
	}
}

// styleColors collects every colour a lipgloss style can carry. Margin
// background has no getter; the enforcement tests already keep any raw colour
// constructor out of theme.go, so it can only come from a role.
func styleColors(s lipgloss.Style) []string {
	cols := []color.Color{
		s.GetForeground(), s.GetBackground(), s.GetUnderlineColor(),
		s.GetBorderTopForeground(), s.GetBorderRightForeground(),
		s.GetBorderBottomForeground(), s.GetBorderLeftForeground(),
		s.GetBorderTopBackground(), s.GetBorderRightBackground(),
		s.GetBorderBottomBackground(), s.GetBorderLeftBackground(),
	}
	cols = append(cols, s.GetBorderForegroundBlend()...)
	out := make([]string, len(cols))
	for i, c := range cols {
		if c == nil {
			continue
		}
		if _, none := c.(lipgloss.NoColor); none {
			continue
		}
		out[i] = HexString(c)
	}
	return out
}

var (
	styleType = reflect.TypeOf(lipgloss.Style{})
	colorType = reflect.TypeOf((*color.Color)(nil)).Elem()
)

// walkColours calls f with the path and colour list of every style and bare
// colour reachable from the exported fields of v, descending into structs.
func walkColours(v reflect.Value, path string, f func(path string, cols []string)) {
	for i := range v.NumField() {
		sf := v.Type().Field(i)
		if !sf.IsExported() {
			continue
		}
		fv, name := v.Field(i), path+sf.Name
		switch {
		case fv.Type() == styleType:
			f(name, styleColors(fv.Interface().(lipgloss.Style)))
		case fv.Type() == colorType:
			if !fv.IsNil() {
				f(name, []string{HexString(fv.Interface().(color.Color))})
			}
		case fv.Kind() == reflect.Struct:
			walkColours(fv, name+".", f)
		}
	}
}

// loudPalette shares no colour with the built-ins.
func loudPalette(t *testing.T) Palette {
	t.Helper()
	var b16 [16]color.Color
	for i := range b16 {
		b16[i] = color.RGBA{R: uint8(255 - i*13), G: uint8(30 + i*11), B: uint8(7 + i*9), A: 255}
	}
	return mustPalette(t)(FromBase16("loud", Dark, b16))
}

// TestNothingIsHardcoded builds themes from two very different palettes and
// checks every colour in every style (all border sides, backgrounds, underline
// colour), every role, every syntax class and the generated Chroma style
// differs. Anything still hardcoded would be identical in both.
func TestNothingIsHardcoded(t *testing.T) {
	a, b := FromPalette(builtinDark), FromPalette(loudPalette(t))
	if a.ID == b.ID {
		t.Fatal("setup: themes share an ID")
	}
	got := map[string][]string{}
	walkColours(reflect.ValueOf(a), "", func(path string, cols []string) { got[path] = cols })
	checked := 0
	walkColours(reflect.ValueOf(b), "", func(path string, cols []string) {
		ca := got[path]
		if len(ca) != len(cols) {
			t.Fatalf("%s: colour lists differ in length", path)
		}
		for i := range ca {
			if ca[i] == "" && cols[i] == "" {
				continue
			}
			checked++
			if ca[i] == cols[i] {
				t.Errorf("%s colour %d is %q in both palettes; is it hardcoded?", path, i, ca[i])
			}
		}
	})
	if checked < 100 {
		t.Errorf("only %d colours compared; the walk is missing styles", checked)
	}

	// Syntax holds its styles privately, one per class.
	for c := ClassKeyword; c < NumSyntaxClasses; c++ {
		ca, cb := styleColors(a.Syntax.For(c)), styleColors(b.Syntax.For(c))
		if ca[0] == "" || ca[0] == cb[0] {
			t.Errorf("syntax class %d colour %q / %q", c, ca[0], cb[0])
		}
	}

	// Every token type the generated Chroma style colours.
	sa, sb := newChromaStyle("a", a.Roles), newChromaStyle("b", b.Roles)
	for _, tok := range sa.Types() {
		ca, cb := sa.Get(tok).Colour, sb.Get(tok).Colour
		if ca.IsSet() && ca == cb {
			t.Errorf("chroma token %v is %s in both palettes", tok, ca)
		}
	}
}

// TestChromaStyleFollowsClasses proves the diff and Markdown share one
// mapping for every Chroma token type, not just the listed ones: Chroma's own
// inheritance must never decide a colour. Plain tokens are the text role in
// the Chroma style and unset in the diff (the pane's own colour). The few
// markup entries (errors, diff markup) are the only exceptions, and no token
// has a background.
func TestChromaStyleFollowsClasses(t *testing.T) {
	for _, th := range []Theme{New(true), New(false)} {
		st := newChromaStyle("t", th.Roles)
		markup := chromaMarkup(th.Roles)
		checked := 0
		for tok := range chroma.StandardTypes {
			entry := st.Get(tok)
			if entry.Background.IsSet() {
				t.Errorf("%s: token %v has a background %s", th.ID, tok, entry.Background)
			}
			if _, ok := markup[tok]; ok || tok == chroma.Background {
				continue
			}
			class := ClassOf(tok)
			want := HexString(th.Roles.syntaxColour(class))
			if got := entry.Colour.String(); got != want {
				t.Errorf("%s: token %v (class %d): chroma colour %s, want %s", th.ID, tok, class, got, want)
			}
			diffFg := styleColors(th.Syntax.For(class))[0]
			if class == ClassPlain {
				if diffFg != "" {
					t.Errorf("token %v is plain but its diff style sets %s", tok, diffFg)
				}
			} else if diffFg != want {
				t.Errorf("%s: token %v (class %d): diff colour %s, want %s", th.ID, tok, class, diffFg, want)
			}
			checked++
		}
		if checked < 50 {
			t.Errorf("only %d token types checked", checked)
		}
	}
}

func TestClassOfInherits(t *testing.T) {
	tests := []struct {
		tok  chroma.TokenType
		want SyntaxClass
	}{
		{chroma.LiteralStringDouble, ClassString},
		{chroma.LiteralStringEscape, ClassEscape},
		{chroma.LiteralNumberInteger, ClassNumber},
		{chroma.KeywordDeclaration, ClassKeyword},
		{chroma.KeywordType, ClassType},
		{chroma.CommentSingle, ClassComment},
		{chroma.CommentPreproc, ClassOperator},
		{chroma.NameFunction, ClassFunction},
		{chroma.NameOther, ClassPlain},
		{chroma.Text, ClassPlain},
	}
	for _, tc := range tests {
		if got := ClassOf(tc.tok); got != tc.want {
			t.Errorf("ClassOf(%v) = %d, want %d", tc.tok, got, tc.want)
		}
	}
}
