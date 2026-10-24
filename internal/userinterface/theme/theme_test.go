package theme

import (
	"bytes"
	"image/color"
	"log/slog"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/google/go-cmp/cmp"
)

func mustPalette(test *testing.T) func(Palette, error) Palette {
	return func(palette Palette, err error) Palette {
		test.Helper()
		if err != nil {
			test.Fatal(err)
		}
		return palette
	}
}

func TestFromBase16Fallbacks(test *testing.T) {
	var blue16 [16]color.Color
	for i := range blue16 {
		blue16[i] = color.RGBA{R: uint8(i * 10), G: uint8(i), B: 7, A: 255}
	}
	palette := mustPalette(test)(FromBase16("x", Light, blue16))
	if palette.Variant != Light || palette.Name != "x" {
		test.Errorf("name/variant = %q/%v", palette.Name, palette.Variant)
	}
	for i := range blue16 {
		if palette.colors[i] != blue16[i] {
			test.Errorf("slot %d = %v, want %v", i, palette.colors[i], blue16[i])
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
	for _, testCase := range tests {
		if palette.colors[testCase.slot] != blue16[testCase.from] {
			test.Errorf("slot %d = %v, want slot %d (%v)", testCase.slot, palette.colors[testCase.slot], testCase.from, blue16[testCase.from])
		}
	}
}

func TestFromBase16MissingSlot(test *testing.T) {
	var blue16 [16]color.Color
	for i := range blue16 {
		blue16[i] = color.Black
	}
	blue16[Base0B] = nil
	_, err := FromBase16("x", Dark, blue16)
	if err == nil || !strings.Contains(err.Error(), "base0B") {
		test.Errorf("err = %v, want one naming base0B", err)
	}
}

func ansiFixture() (background, foreground color.Color, ansi [16]color.Color) {
	names := []string{
		"#000000", "#cc0000", "#00cc00", "#cccc00", "#0000cc", "#cc00cc", "#00cccc", "#cccccc",
		"#555555", "#ff5555", "#55ff55", "#ffff55", "#5555ff", "#ff55ff", "#55ffff", "#ffffff",
	}
	for i, name := range names {
		ansi[i] = hex(name)
	}
	return hex("#101010"), hex("#e0e0e0"), ansi
}

func TestFromANSI(test *testing.T) {
	background, foreground, ansi := ansiFixture()
	palette := mustPalette(test)(FromANSI("term", background, foreground, ansi))
	if palette.Variant != Dark {
		test.Errorf("dark background gave variant %v", palette.Variant)
	}
	direct := []struct {
		slot Slot
		want color.Color
	}{
		{Base00, background},
		{Base03, ansi[8]}, // bright black: the comment colour terminals pick
		{Base05, foreground},
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
	for _, testCase := range direct {
		if got := palette.colors[testCase.slot]; got != testCase.want {
			test.Errorf("slot %s = %s, want %s", testCase.slot, HexString(got), HexString(testCase.want))
		}
	}
	if got, want := HexString(palette.colors[Base09]), "#cc6600"; got != want {
		test.Errorf("orange = %s, want %s (red blended with yellow)", got, want)
	}
	// The blended shades sit between background and foreground, in order.
	previous := luminance(palette.colors[Base00])
	for _, slot := range []Slot{Base01, Base02, Base04, Base05, Base06, Base07} {
		lightness := luminance(palette.colors[slot])
		if lightness <= previous {
			test.Errorf("slot %s (luminance %.3f) is not brighter than the one before (%.3f)", slot, lightness, previous)
		}
		previous = lightness
	}
	if err := palette.check(); err != nil {
		test.Error(err)
	}
}

func TestFromANSIMutedShade(test *testing.T) {
	_, _, ansi := ansiFixture()
	blended := func(background, foreground color.Color) string { return HexString(blend(background, foreground, 0.45)) }
	tests := []struct {
		name                   string
		background, foreground string
		bright                 string
		wantBlend              bool
	}{
		{"distinct bright black is used", "#101010", "#e0e0e0", "#555555", false},
		// Solarized Dark: color8 is the background.
		{"bright black equals the background", "#002b36", "#839496", "#002b36", true},
		{"bright black equals the foreground", "#101010", "#e0e0e0", "#e0e0e0", true},
		{"bright black almost the background", "#101010", "#e0e0e0", "#121212", true},
	}
	for _, testCase := range tests {
		colour := ansi
		colour[8] = hex(testCase.bright)
		palette := mustPalette(test)(FromANSI("term", hex(testCase.background), hex(testCase.foreground), colour))
		want := testCase.bright
		if testCase.wantBlend {
			want = blended(hex(testCase.background), hex(testCase.foreground))
		}
		if got := HexString(palette.colors[Base03]); got != want {
			test.Errorf("%s: base03 = %s, want %s", testCase.name, got, want)
		}
	}
}

func TestFromANSIVariant(test *testing.T) {
	_, _, ansi := ansiFixture()
	tests := []struct {
		name       string
		background string
		want       Variant
		far07      color.Color
	}{
		{"white", "#fafafa", Light, ansi[0]},
		{"black", "#101010", Dark, ansi[15]},
		// Black reads better than white on mid grey; a 0.5 luminance cut-off
		// would call this dark.
		{"mid grey", "#9a9a9a", Light, ansi[0]},
	}
	for _, testCase := range tests {
		palette := mustPalette(test)(FromANSI("term", hex(testCase.background), hex("#202020"), ansi))
		if palette.Variant != testCase.want {
			test.Errorf("%s: variant = %v, want %v", testCase.name, palette.Variant, testCase.want)
		}
		if palette.colors[Base07] != testCase.far07 {
			test.Errorf("%s: base07 = %s, want %s", testCase.name, HexString(palette.colors[Base07]), HexString(testCase.far07))
		}
	}
}

func TestFromANSIMissingInput(test *testing.T) {
	background, foreground, ansi := ansiFixture()
	if _, err := FromANSI("t", nil, foreground, ansi); err == nil {
		test.Error("nil background accepted")
	}
	if _, err := FromANSI("t", background, nil, ansi); err == nil {
		test.Error("nil foreground accepted")
	}
	ansi[11] = nil
	_, err := FromANSI("t", background, foreground, ansi)
	if err == nil || !strings.Contains(err.Error(), "11") {
		test.Errorf("err = %v, want one naming ANSI colour 11", err)
	}
}

func TestParseHex(test *testing.T) {
	for _, text := range []string{"#1a2b3c", "#FFFFFF"} {
		if _, err := ParseHex(text); err != nil {
			test.Errorf("ParseHex(%q) = %v", text, err)
		}
	}
	for _, text := range []string{"", "red", "#fff", "#12345", "#1234567", "#gggggg", "1a2b3c", " #1a2b3c"} {
		if _, err := ParseHex(text); err == nil {
			test.Errorf("ParseHex(%q) succeeded", text)
		}
	}
	defer func() {
		if recover() == nil {
			test.Error("hex of a malformed literal did not panic")
		}
	}()
	hex("nope")
}

func TestBuiltinPalettesComplete(test *testing.T) {
	for _, palette := range []Palette{builtinDark, builtinLight} {
		if err := palette.check(); err != nil {
			test.Error(err)
		}
	}
	if New(true).Identifier == New(false).Identifier {
		test.Error("dark and light share an ID")
	}
}

func TestVariants(test *testing.T) {
	if !New(true).IsDark || New(false).IsDark || !Default().IsDark {
		test.Error("IsDark does not follow the palette variant")
	}
	if diff := cmp.Diff(New(true).Identifier, FromPalette(builtinDark).Identifier); diff != "" {
		test.Errorf("New(true) is not the dark built-in: %s", diff)
	}
}

// TestBuiltinPalettesPassGuard proves the contrast guard leaves the built-in
// palettes untouched: they were chosen to be readable.
func TestBuiltinPalettesPassGuard(test *testing.T) {
	for _, palette := range []Palette{builtinDark, builtinLight} {
		raw := defaultRoles(palette)
		guarded := raw
		guarded.guard(nil)
		if diff := cmp.Diff(colorHexes(raw), colorHexes(guarded)); diff != "" {
			test.Errorf("%s: the guard changed roles (-derived +guarded):\n%s", palette.Name, diff)
		}
	}
}

// TestBuiltinPalettesGuardQuietly pins that the built-in themes trigger no
// contrast-guard warnings.
func TestBuiltinPalettesGuardQuietly(test *testing.T) {
	buffer := captureLog(test)
	New(true)
	New(false)
	if buffer.Len() != 0 {
		test.Errorf("built-in themes logged: %s", buffer.String())
	}
}

func colorHexes(roles Roles) map[string]string {
	out := map[string]string{}
	for _, field := range roles.fields() {
		out[field.name] = HexString(*field.color)
	}
	return out
}

// deltaEOK is the Euclidean distance between two colours in OKLab.
func deltaEOK(first, second color.Color) float64 {
	firstOklch, secondOklch := toOKLCH(first), toOKLCH(second)
	deltaA := firstOklch.chroma*math.Cos(firstOklch.hue) - secondOklch.chroma*math.Cos(secondOklch.hue)
	deltaB := firstOklch.chroma*math.Sin(firstOklch.hue) - secondOklch.chroma*math.Sin(secondOklch.hue)
	return math.Sqrt((firstOklch.lightness-secondOklch.lightness)*(firstOklch.lightness-secondOklch.lightness) + deltaA*deltaA + deltaB*deltaB)
}

// TestBuiltinTintsMatchTheEarlierLook checks the derived diff tints stay
// within a small perceptual distance (OKLab delta E) of the hand-picked
// colours they replaced.
func TestBuiltinTintsMatchTheEarlierLook(test *testing.T) {
	const maximumDeltaE = 0.02
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
	count := 0
	for _, palette := range []Palette{builtinDark, builtinLight} {
		roles := deriveRoles(palette, Roles{})
		for _, field := range roles.fields() {
			want, ok := old[palette.Name][field.name]
			if !ok {
				continue
			}
			count++
			delta := deltaEOK(*field.color, hex(want))
			test.Logf("%s %-20s %s vs %s: dE_OK = %.4f", palette.Name, field.name, HexString(*field.color), want, delta)
			if delta > maximumDeltaE {
				test.Errorf("%s %s = %s, dE_OK %.4f from %s, want at most %.2f", palette.Name, field.name, HexString(*field.color), delta, want, maximumDeltaE)
			}
		}
	}
	if count != 12 {
		test.Errorf("compared %d tints, want 12", count)
	}
}

func TestGuardFixesUnreadableDerivedRoles(test *testing.T) {
	bad := builtinDark
	bad.colors[Base05] = bad.colors[Base00] // text invisible on the page
	bad.colors[Base03] = bad.colors[Base01] // muted invisible on the bar
	bad.colors[Base0B] = bad.colors[Base00] // success invisible
	roles := deriveRoles(bad, Roles{})
	checks := []struct {
		name       string
		foreground color.Color
		background color.Color
		minimum    float64
	}{
		{"text", roles.Text, roles.Background, minimumTextContrast},
		{"text on bar", roles.Text, roles.Surface, minimumTextContrast},
		{"muted", roles.Muted, roles.Background, minimumAccentContrast},
		{"muted on bar", roles.Muted, roles.Surface, minimumAccentContrast},
		{"success", roles.Success, roles.Background, minimumAccentContrast},
	}
	for _, testCase := range checks {
		if got := contrast(testCase.foreground, testCase.background); got < testCase.minimum {
			test.Errorf("%s contrast = %.2f, want at least %.1f", testCase.name, got, testCase.minimum)
		}
	}
}

func TestGuardLightBackgroundGoesDark(test *testing.T) {
	bad := builtinLight
	bad.colors[Base05] = bad.colors[Base00]
	roles := deriveRoles(bad, Roles{})
	if luminance(roles.Text) >= luminance(roles.Background) {
		test.Errorf("text %s was not darkened against a light background", HexString(roles.Text))
	}
}

func TestNudgePicksTheBetterExtreme(test *testing.T) {
	// On mid grey, black has far more contrast than white, although the
	// background's luminance is under 0.5.
	background := hex("#a0a0a0")
	if got := contrast(color.Black, background); got < 4.5 {
		test.Fatalf("setup: black on %s = %.2f", HexString(background), got)
	}
	out := nudge(hex("#909090"), background, 4.5)
	if got := contrast(out, background); got < 4.5 {
		test.Errorf("nudged to %s with contrast %.2f, want 4.5", HexString(out), got)
	}
	if luminance(out) >= luminance(background) {
		test.Errorf("nudged toward white (%s)", HexString(out))
	}
}

// captureLog sends slog's default logger to a buffer for the test.
func captureLog(test *testing.T) *bytes.Buffer {
	test.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

func TestOverridesApplyAfterDerivation(test *testing.T) {
	overrides := Roles{Added: hex("#00ff00"), HunkTint: hex("#123456")}
	roles := deriveRoles(builtinDark, overrides)
	if got := HexString(roles.Added); got != "#00ff00" {
		test.Errorf("Added = %s, want the override", got)
	}
	if got := HexString(roles.HunkTint); got != "#123456" {
		test.Errorf("HunkTint = %s, want the override", got)
	}
	// An unset tint follows the overridden role it blends.
	base := deriveRoles(builtinDark, Roles{})
	if HexString(roles.AddedTint) == HexString(base.AddedTint) {
		test.Error("AddedTint ignored the Added override")
	}
	if HexString(roles.RemovedTint) != HexString(base.RemovedTint) {
		test.Error("RemovedTint changed without an override")
	}
}

// TestOverridesAreExact: the guard never changes an override, it only logs.
// The values are Tokyo Night's background and muted text, which sit at 2.76:1.
func TestOverridesAreExact(test *testing.T) {
	buffer := captureLog(test)
	overrides := Roles{Background: hex("#1a1b26"), Muted: hex("#565f89")}
	overridden := deriveRoles(builtinDark, overrides)
	if got := HexString(overridden.Muted); got != "#565f89" {
		test.Errorf("Muted = %s, want the exact override #565f89", got)
	}
	log := buffer.String()
	if strings.Count(log, "level=WARN") != 1 || !strings.Contains(log, "role=muted") || !strings.Contains(log, "contrast=") {
		test.Errorf("want one warning naming the muted role and its ratio, got:\n%s", log)
	}

	// The same value, derived rather than overridden, is nudged and not logged.
	bad := builtinDark
	bad.colors[Base03] = hex("#565f89")
	bad.colors[Base00] = hex("#1a1b26")
	buffer.Reset()
	derived := deriveRoles(bad, Roles{})
	if HexString(derived.Muted) == "#565f89" {
		test.Error("a derived muted role below the threshold was not nudged")
	}
	if buffer.Len() != 0 {
		test.Errorf("derived roles logged: %s", buffer.String())
	}
}

func TestOverrideChangesIdentity(test *testing.T) {
	base := FromPalette(builtinDark)
	over := FromPaletteWithOverrides(builtinDark, Roles{SyntaxString: hex("#ff00ff")})
	if base.Identifier == over.Identifier {
		test.Fatal("a role override did not change Theme.Identifier")
	}
	if base.ChromaStyleName() == over.ChromaStyleName() {
		test.Fatal("a role override did not change the Chroma style name")
	}
	got := over.Syntax.chroma.Get(chroma.LiteralString).Colour.String()
	if got != "#ff00ff" {
		test.Errorf("string colour in the overridden Chroma style = %s, want #ff00ff", got)
	}
	if FromPalette(builtinDark).Identifier != base.Identifier {
		test.Error("ID is not stable for the same inputs")
	}
}

// styleColors collects every colour a lipgloss style can carry. Margin
// background has no getter; the enforcement tests already keep any raw colour
// constructor out of theme.go, so it can only come from a role.
func styleColors(style lipgloss.Style) []string {
	columns := []color.Color{
		style.GetForeground(), style.GetBackground(), style.GetUnderlineColor(),
		style.GetBorderTopForeground(), style.GetBorderRightForeground(),
		style.GetBorderBottomForeground(), style.GetBorderLeftForeground(),
		style.GetBorderTopBackground(), style.GetBorderRightBackground(),
		style.GetBorderBottomBackground(), style.GetBorderLeftBackground(),
	}
	columns = append(columns, style.GetBorderForegroundBlend()...)
	out := make([]string, len(columns))
	for i, colour := range columns {
		if colour == nil {
			continue
		}
		if _, none := colour.(lipgloss.NoColor); none {
			continue
		}
		out[i] = HexString(colour)
	}
	return out
}

var (
	styleType = reflect.TypeOf(lipgloss.Style{})
	colorType = reflect.TypeOf((*color.Color)(nil)).Elem()
)

// walkColours calls f with the path and colour list of every style and bare
// colour reachable from the exported fields of value, descending into structs.
func walkColours(value reflect.Value, path string, check func(path string, columns []string)) {
	for i := range value.NumField() {
		structField := value.Type().Field(i)
		if !structField.IsExported() {
			continue
		}
		fieldValue, name := value.Field(i), path+structField.Name
		switch {
		case fieldValue.Type() == styleType:
			check(name, styleColors(fieldValue.Interface().(lipgloss.Style)))
		case fieldValue.Type() == colorType:
			if !fieldValue.IsNil() {
				check(name, []string{HexString(fieldValue.Interface().(color.Color))})
			}
		case fieldValue.Kind() == reflect.Struct:
			walkColours(fieldValue, name+".", check)
		}
	}
}

// loudPalette shares no colour with the built-ins.
func loudPalette(test *testing.T) Palette {
	test.Helper()
	var blue16 [16]color.Color
	for i := range blue16 {
		blue16[i] = color.RGBA{R: uint8(255 - i*13), G: uint8(30 + i*11), B: uint8(7 + i*9), A: 255}
	}
	return mustPalette(test)(FromBase16("loud", Dark, blue16))
}

// TestNothingIsHardcoded builds themes from two very different palettes and
// checks every colour in every style (all border sides, backgrounds, underline
// colour), every role, every syntax class and the generated Chroma style
// differs. Anything still hardcoded would be identical in both.
func TestNothingIsHardcoded(test *testing.T) {
	first, theme := FromPalette(builtinDark), FromPalette(loudPalette(test))
	if first.Identifier == theme.Identifier {
		test.Fatal("setup: themes share an ID")
	}
	got := map[string][]string{}
	walkColours(reflect.ValueOf(first), "", func(path string, columns []string) { got[path] = columns })
	checked := 0
	walkColours(reflect.ValueOf(theme), "", func(path string, columns []string) {
		actual := got[path]
		if len(actual) != len(columns) {
			test.Fatalf("%s: colour lists differ in length", path)
		}
		for i := range actual {
			if actual[i] == "" && columns[i] == "" {
				continue
			}
			checked++
			if actual[i] == columns[i] {
				test.Errorf("%s colour %d is %q in both palettes; is it hardcoded?", path, i, actual[i])
			}
		}
	})
	if checked < 100 {
		test.Errorf("only %d colours compared; the walk is missing styles", checked)
	}

	// Syntax holds its styles privately, one per token type.
	for _, token := range first.Syntax.TokenTypes() {
		if ClassOf(token) == ClassPlain {
			continue // the pane's own text colour
		}
		actual, second := styleColors(first.Syntax.For(token)), styleColors(theme.Syntax.For(token))
		if actual[0] == "" || actual[0] == second[0] {
			test.Errorf("syntax token %v colour %q / %q", token, actual[0], second[0])
		}
	}

	// Every token type the Chroma style colours.
	firstStyle, secondStyle := first.Syntax.chroma, theme.Syntax.chroma
	for _, token := range firstStyle.Types() {
		first, second := firstStyle.Get(token).Colour, secondStyle.Get(token).Colour
		if first.IsSet() && first == second {
			test.Errorf("chroma token %v is %s in both palettes", token, first)
		}
	}
}

// TestGeneratedStyleFollowsClasses proves the generated style colours every
// Chroma token type by its class, not just the listed ones: Chroma's own
// inheritance must never decide a colour. The few markup entries (errors,
// diff markup) are the only exceptions.
func TestGeneratedStyleFollowsClasses(test *testing.T) {
	for _, theme := range []Theme{New(true), New(false)} {
		markup := chromaMarkup(theme.Roles)
		checked := 0
		for _, token := range standardTypes() {
			if _, ok := markup[token]; ok {
				continue
			}
			class := ClassOf(token)
			want := HexString(theme.Roles.syntaxColour(class))
			if got := theme.Syntax.chroma.Get(token).Colour.String(); got != want {
				test.Errorf("%s: token %v (class %d): chroma colour %s, want %s", theme.Identifier, token, class, got, want)
			}
			checked++
		}
		if checked < 50 {
			test.Errorf("only %d token types checked", checked)
		}
	}
}

// TestDiffPaneMatchesChromaStyle proves the diff pane and Markdown code
// blocks draw every standard token type alike, for a generated and a named
// style: the same colour, bold, italic and underline, and never a background.
// A token the diff leaves at the pane's own colour is the text role in Chroma.
// For the named style, the colour is also the bundled style's own, unless that
// is unreadable on the page and was nudged.
func TestDiffPaneMatchesChromaStyle(test *testing.T) {
	named, err := FromPaletteWithSyntax(builtinDark, Roles{}, "tokyonight-night")
	if err != nil {
		test.Fatal(err)
	}
	for _, theme := range []Theme{New(true), New(false), named} {
		checked := 0
		for _, token := range standardTypes() {
			entry, diffStyle := theme.Syntax.chroma.Get(token), theme.Syntax.For(token)
			if entry.Background.IsSet() || styleColors(diffStyle)[1] != "" {
				test.Errorf("%q: token %v has a background", theme.Syntax.Name(), token)
			}
			diffForeground := styleColors(diffStyle)[0]
			if diffForeground == "" {
				diffForeground = HexString(theme.Roles.Text)
			}
			if got := entry.Colour.String(); got != diffForeground {
				test.Errorf("%q: token %v: chroma colour %s, diff colour %s", theme.Syntax.Name(), token, got, diffForeground)
			}
			if original := bundledChromaStyles[theme.Syntax.Name()]; original != nil && original.Get(token).Colour.IsSet() {
				want := original.Get(token).Colour.String()
				readable := contrast(mustParseHex(want), theme.Roles.Background) >= minimumAccentContrast
				if readable && diffForeground != want {
					test.Errorf("%q: token %v: diff colour %s, the style's own is %s", theme.Syntax.Name(), token, diffForeground, want)
				}
			}
			for _, attribute := range []struct {
				name   string
				chroma chroma.Trilean
				diff   bool
			}{
				{"bold", entry.Bold, diffStyle.GetBold()},
				{"italic", entry.Italic, diffStyle.GetItalic()},
				{"underline", entry.Underline, diffStyle.GetUnderline()},
			} {
				if (attribute.chroma == chroma.Yes) != attribute.diff {
					test.Errorf("%q: token %v: %s is %v in chroma, %v in the diff", theme.Syntax.Name(), token, attribute.name, attribute.chroma, attribute.diff)
				}
			}
			checked++
		}
		if checked < 50 {
			test.Errorf("only %d token types checked", checked)
		}
	}
}

func TestClassOfInherits(test *testing.T) {
	tests := []struct {
		token chroma.TokenType
		want  SyntaxClass
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
	for _, testCase := range tests {
		if got := ClassOf(testCase.token); got != testCase.want {
			test.Errorf("ClassOf(%v) = %d, want %d", testCase.token, got, testCase.want)
		}
	}
}

func namedTheme(test *testing.T, palette Palette, overrides Roles, syntax string) Theme {
	test.Helper()
	built, err := FromPaletteWithSyntax(palette, overrides, syntax)
	if err != nil {
		test.Fatal(err)
	}
	return built
}

// TestSyntaxOverrideRecoloursClass proves a syn-<class> role recolours every
// standard token type in its class, over a generated and a named style, and
// leaves the other classes alone.
func TestSyntaxOverrideRecoloursClass(test *testing.T) {
	const want = "#ff00ff"
	for _, syntax := range []string{"", "tokyonight-night", "monokai"} {
		before := namedTheme(test, builtinDark, Roles{}, syntax)
		after := namedTheme(test, builtinDark, Roles{SyntaxString: hex(want)}, syntax)
		checked := 0
		for _, token := range standardTypes() {
			got := styleColors(after.Syntax.For(token))[0]
			if ClassOf(token) == ClassString {
				checked++
				if got != want {
					test.Errorf("%q: string token %v is %s, want the override %s", syntax, token, got, want)
				}
				continue
			}
			if previous := styleColors(before.Syntax.For(token))[0]; got != previous {
				test.Errorf("%q: non-string token %v changed from %s to %s", syntax, token, previous, got)
			}
		}
		if checked < 10 {
			test.Errorf("%q: only %d string token types checked", syntax, checked)
		}
	}
}

// TestNamedStyleIsGuarded puts a dark style on a light page: every colour it
// draws must still be readable, and the unreadable ones are logged once each.
func TestNamedStyleIsGuarded(test *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })

	light := namedTheme(test, builtinLight, Roles{}, "tokyonight-night")
	for _, token := range standardTypes() {
		foreground := light.Syntax.For(token).GetForeground()
		if ratio := contrast(foreground, light.Roles.Background); ratio < minimumAccentContrast {
			test.Errorf("token %v: %s has contrast %.2f on the page", token, HexString(foreground), ratio)
		}
	}
	logged := buffer.String()
	if !strings.Contains(logged, "syntax style colour is hard to read") || !strings.Contains(logged, "colour=#c0caf5") {
		test.Errorf("no warning for tokyonight-night's text colour: %s", logged)
	}
	if count := strings.Count(logged, "colour=#c0caf5"); count != 1 {
		test.Errorf("#c0caf5 was logged %d times, want once", count)
	}
}

// TestNamedStyleOverrideIsExact proves a syn-* override is used as written
// even when it is hard to read, and that the style colour it replaces is
// neither nudged nor logged.
func TestNamedStyleOverrideIsExact(test *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })

	page := builtinDark
	page.colors[Base00] = hex("#1a1b26")
	const faint = "#565f89" // 2.76:1 on #1a1b26
	// tokyonight-night draws comments, doc strings and preprocessor lines
	// (operator class) #414868; override all three.
	built := namedTheme(test, page, Roles{SyntaxComment: hex(faint), SyntaxString: hex("#9ece6a"), SyntaxOperator: hex("#89ddff")}, "tokyonight-night")
	for _, token := range standardTypes() {
		if ClassOf(token) != ClassComment {
			continue
		}
		if got := HexString(built.Syntax.For(token).GetForeground()); got != faint {
			test.Errorf("comment token %v is %s, want the override %s as written", token, got, faint)
		}
	}
	if logged := buffer.String(); strings.Contains(logged, "#414868") {
		test.Errorf("the replaced comment colour was logged: %s", logged)
	}
}

// TestNamedStyleSkipsMetaTypes proves the line-number and wrapper types, which
// strig never draws, neither get entries nor trigger warnings.
func TestNamedStyleSkipsMetaTypes(test *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })

	built := namedTheme(test, builtinDark, Roles{}, "onedark") // its line numbers, #55595f, are faint
	if logged := buffer.String(); logged != "" {
		test.Errorf("onedark logged warnings: %s", logged)
	}
	for _, token := range []chroma.TokenType{chroma.Background, chroma.LineNumbers, chroma.LineHighlight, chroma.CodeLine} {
		if _, ok := built.Syntax.styles[token]; ok {
			test.Errorf("the syntax table has an entry for %v", token)
		}
	}
	if _, ok := built.Syntax.styles[chroma.Error]; !ok {
		test.Error("the syntax table lost its Error entry")
	}
}

func TestSyntaxStyleChangesIdentity(test *testing.T) {
	generated := namedTheme(test, builtinDark, Roles{}, "")
	night := namedTheme(test, builtinDark, Roles{}, "tokyonight-night")
	storm := namedTheme(test, builtinDark, Roles{}, "tokyonight-storm")
	if generated.Identifier == night.Identifier || night.Identifier == storm.Identifier {
		test.Error("the syntax style is not part of Theme.Identifier")
	}
	if night.ChromaStyleName() == storm.ChromaStyleName() {
		test.Error("two syntax styles share a Chroma style name")
	}
	if generated.Identifier != FromPalette(builtinDark).Identifier {
		test.Error("an empty syntax name is not the generated style")
	}

	// Overriding a role with the colour it already had changes no role, but
	// it recolours the named style's whole class, so the identity must change.
	same := namedTheme(test, builtinDark, Roles{SyntaxString: night.Roles.SyntaxString}, "tokyonight-night")
	if HexString(same.Syntax.For(chroma.LiteralString).GetForeground()) == HexString(night.Syntax.For(chroma.LiteralString).GetForeground()) {
		test.Fatal("setup: the override did not recolour strings")
	}
	if same.Identifier == night.Identifier || same.ChromaStyleName() == night.ChromaStyleName() {
		test.Error("an override equal to the derived colour did not change Theme.Identifier")
	}
	if upper := namedTheme(test, builtinDark, Roles{}, "TokyoNight-Night"); upper.Identifier != night.Identifier {
		test.Error("the style name's case changed Theme.Identifier")
	}
}

func TestSyntaxStyleNameIgnoresCase(test *testing.T) {
	if err := CheckSyntaxStyle("Dracula"); err != nil {
		test.Error(err)
	}
	if name := namedTheme(test, builtinDark, Roles{}, "Dracula").Syntax.Name(); name != "dracula" {
		test.Errorf("Syntax.Name() = %q, want dracula", name)
	}
}

func TestUnknownSyntaxStyle(test *testing.T) {
	_, err := FromPaletteWithSyntax(builtinDark, Roles{}, "nope")
	if err == nil {
		test.Fatal("no error")
	}
	for _, want := range []string{`unknown syntax style "nope"`, "monokai", "tokyonight-night"} {
		if !strings.Contains(err.Error(), want) {
			test.Errorf("error %q lacks %q", err, want)
		}
	}
	if slices.ContainsFunc(SyntaxStyleNames(), func(name string) bool { return strings.HasPrefix(name, "strig-") }) {
		test.Error("SyntaxStyleNames lists a style strig generated")
	}
}

// TestNamedStyleWarningsAreStable proves a style that logs several warnings
// logs them in the same order every time, so goldens that record them hold.
func TestNamedStyleWarningsAreStable(test *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{ReplaceAttr: func(_ []string, attribute slog.Attr) slog.Attr {
		if attribute.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return attribute
	}})))
	test.Cleanup(func() { slog.SetDefault(previous) })

	build := func() string {
		buffer.Reset()
		namedTheme(test, builtinLight, Roles{}, "tokyonight-night")
		return buffer.String()
	}
	first := build()
	if strings.Count(first, "syntax style colour is hard to read") < 3 {
		test.Fatalf("setup: want several warnings, got %s", first)
	}
	for range 20 {
		if again := build(); again != first {
			test.Fatalf("warnings changed order:\n%s\nthen\n%s", first, again)
		}
	}
}

// TestNamedStyleWithoutTextColour uses a style that gives plain code no
// colour: plain tokens keep the pane's own colour (the text role, here an
// exact override that is hard to read), and nothing blames the style for it.
func TestNamedStyleWithoutTextColour(test *testing.T) {
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })

	const faint = "#565f89"
	if bundledChromaStyles["github"].Get(chroma.Text).Colour.IsSet() {
		test.Fatal("setup: github now colours plain text")
	}
	built := namedTheme(test, builtinDark, Roles{Text: hex(faint)}, "github")
	if foreground := styleColors(built.Syntax.For(chroma.Text))[0]; foreground != "" {
		test.Errorf("plain text has foreground %s in the diff, want the pane's own", foreground)
	}
	if got := built.Syntax.chroma.Get(chroma.Text).Colour.String(); got != faint {
		test.Errorf("plain text is %s in Chroma, want the text role %s", got, faint)
	}
	if logged := buffer.String(); strings.Contains(logged, faint) && strings.Contains(logged, "syntax style colour") {
		test.Errorf("the style was blamed for the text role: %s", logged)
	}
}
