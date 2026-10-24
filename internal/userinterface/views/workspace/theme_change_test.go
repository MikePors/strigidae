package workspace

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
)

// foreground is the SGR parameters that draw text in colour.
func foreground(colour color.Color) string {
	red, green, blue, _ := colour.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", red>>8, green>>8, blue>>8)
}

// TestThemeChangeRecolours checks that after a ThemeMessage (as `:theme`
// sends) the keywords in the description's Go code block and in the client.go
// diff are drawn in the new theme's keyword colour, not the old one's, so no
// cached rendering survives the change.
func TestThemeChangeRecolours(test *testing.T) {
	tokyoNight, err := scheme.LoadBundled("tokyo-night")
	if err != nil {
		test.Fatal(err)
	}
	next := tokyoNight.Theme()
	old := theme.New(true)
	if foreground(old.Roles.SyntaxKeyword) == foreground(next.Roles.SyntaxKeyword) {
		test.Fatal("the two themes share a keyword colour; pick another")
	}

	views := map[string]func(test *testing.T) *harness{
		"code block in the description": func(test *testing.T) *harness { return loaded(test, 100, 60) },
		"diff line":                     func(test *testing.T) *harness { return openClientDiff(test, 100, 40) },
	}
	for name, open := range views {
		test.Run(name, func(test *testing.T) {
			harness := open(test)
			if before := harness.render(); !strings.Contains(before, foreground(old.Roles.SyntaxKeyword)) {
				test.Fatal("before the change, no keyword is drawn in the old theme's colour")
			}
			harness.send(userinterface.ThemeMessage{Theme: next})
			after := harness.render()
			if !strings.Contains(after, foreground(next.Roles.SyntaxKeyword)) {
				test.Error("after the change, no keyword is drawn in the new theme's colour")
			}
			if strings.Contains(after, foreground(old.Roles.SyntaxKeyword)) {
				test.Error("after the change, something is still drawn in the old theme's keyword colour")
			}
		})
	}
}
