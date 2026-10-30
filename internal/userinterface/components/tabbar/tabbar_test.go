package tabbar

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestRender(test *testing.T) {
	for _, testCase := range []struct {
		name   string
		names  []string
		active int
		width  int
		want   string
	}{
		{"all fit", []string{"All", "Me"}, 0, 40, " All   Me "},
		{"drops tabs before the active one", []string{"Alpha", "Beta", "Gamma"}, 2, 9, " Gamma "},
		{"cuts the end", []string{"Alpha", "Beta"}, 0, 5, " Alph"},
		{"hostile name is cleaned", []string{"a\x1b[31mb"}, 0, 20, " a [31mb "},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			got := ansi.Strip(Render(theme.Default(), testCase.names, testCase.active, testCase.width))
			if got != testCase.want {
				test.Errorf("got %q, want %q", got, testCase.want)
			}
		})
	}
}
