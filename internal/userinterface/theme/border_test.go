package theme

import (
	"image/color"
	"strings"
	"testing"
)

func TestBorderRole(test *testing.T) {
	derived := FromPalette(builtinDark)
	if HexString(derived.Roles.Border) != HexString(derived.Roles.Muted) {
		test.Errorf("border = %s, want it to follow muted (%s)", HexString(derived.Roles.Border), HexString(derived.Roles.Muted))
	}

	overridden := FromPaletteWithOverrides(builtinDark, Roles{Border: hex("#123456")})
	if got := HexString(overridden.PaneBorder.GetForeground()); got != "#123456" {
		test.Errorf("pane border = %s, want the border role #123456", got)
	}
	if HexString(overridden.Roles.Muted) != HexString(derived.Roles.Muted) {
		test.Error("overriding border changed muted")
	}
	if HexString(overridden.Muted.GetForeground()) == "#123456" {
		test.Error("muted text takes the border colour")
	}
	if overridden.Identifier == derived.Identifier {
		test.Error("a border override does not change the theme identifier")
	}

	followed := FromPaletteWithOverrides(builtinDark, Roles{Muted: hex("#abcdef")})
	if got := HexString(followed.PaneBorder.GetForeground()); got != "#abcdef" {
		test.Errorf("pane border = %s, want it to follow an overridden muted", got)
	}
}

func TestRolesSet(test *testing.T) {
	var roles Roles
	if err := roles.Set("border", color.White); err != nil || roles.Border == nil {
		test.Fatalf("Set(border) = %v, border %v", err, roles.Border)
	}
	err := roles.Set("nonsense", color.White)
	if err == nil {
		test.Fatal("unknown role accepted")
	}
	for _, name := range []string{`"nonsense"`, "valid roles:", "border", "added-tint", "syn-escape"} {
		if !strings.Contains(err.Error(), name) {
			test.Errorf("error %q lacks %s", err, name)
		}
	}
}

func TestFromBase24AndVariantOf(test *testing.T) {
	var colours [24]color.Color
	for index := range colours {
		colours[index] = color.Gray{Y: 128}
	}
	if _, err := FromBase24("ok", Dark, colours); err != nil {
		test.Errorf("complete palette: %v", err)
	}
	colours[17] = nil
	if _, err := FromBase24("bad", Dark, colours); err == nil || !strings.Contains(err.Error(), "base11") {
		test.Errorf("missing slot error = %v, want it to name base11", err)
	}
	if VariantOf(hex("#101010")) != Dark || VariantOf(hex("#f0f0f0")) != Light {
		test.Error("VariantOf does not follow the background")
	}
}
