package dashboard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

func infoTexts(view *View, sequence string) []string {
	var texts []string
	for _, message := range messages(press(view, sequence)) {
		if info, ok := message.(userinterface.InfoMessage); ok {
			texts = append(texts, info.Text)
		}
	}
	return texts
}

// searchedView searches "acme", which matches rows 0 and 1, and leaves the
// cursor on row 0.
func searchedView(test *testing.T) *View {
	test.Helper()
	view, _ := loadedView(test)
	press(view, "/")
	promptResult(view, "acme", userinterface.PromptChanged)
	promptResult(view, "acme", userinterface.PromptSubmitted)
	return view
}

func TestHighlightSearchOptionHidesAndRestoresTheMarks(test *testing.T) {
	view := searchedView(test)
	press(view, "n") // row 1, so row 0 is a match without the cursor
	shown := func() bool { return strings.Contains(ansi.Strip(view.Render(80, 16)), search.Glyph) }
	if !shown() {
		test.Fatal("marks are not drawn by default")
	}
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.HighlightSearch: false}})
	if shown() {
		test.Error("nohlsearch still draws the marks")
	}
	press(view, "N")
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("N with nohlsearch: %s", got)
	}
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.HighlightSearch: true}})
	if !shown() {
		test.Error("hlsearch did not bring the marks back")
	}
}

func TestWrapScanOptionDecidesWhetherNAndNWrap(test *testing.T) {
	view := searchedView(test)
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.WrapScan: false}})

	if got := infoTexts(view, "N"); len(got) != 1 || got[0] != "no more matches above" {
		test.Errorf("N on the first match said %q", got)
	}
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("N on the first match moved to %s", got)
	}
	press(view, "n")
	if got := infoTexts(view, "n"); len(got) != 1 || got[0] != "no more matches below" {
		test.Errorf("n on the last match said %q", got)
	}
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("n on the last match moved to %s", got)
	}

	view.Update(userinterface.OptionsMessage{Values: options.Values{options.WrapScan: true}})
	press(view, "n")
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("n with wrapscan: %s, want it wrapped", got)
	}
}
