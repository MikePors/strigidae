package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

func (harness *harness) setOptions(values options.Values) {
	harness.test.Helper()
	harness.send(userinterface.OptionsMessage{Values: values})
}

func TestHighlightSearchOptionHidesAndRestoresTheFilesMarks(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted) // rows 2 and 4 match; cursor on 2
	shown := func() bool {
		return strings.Contains(ansi.Strip(strings.Join(harness.view.fileRows(40, 10), "\n")), search.Glyph)
	}
	if !shown() {
		test.Fatal("marks are not drawn by default")
	}
	harness.setOptions(options.Values{options.HighlightSearch: false})
	if shown() {
		test.Error("nohlsearch still draws the marks")
	}
	harness.press("n")
	if got := harness.view.selection(); got != 4 {
		test.Errorf("n with nohlsearch: %d, want 4", got)
	}
	harness.setOptions(options.Values{options.HighlightSearch: true})
	if !shown() {
		test.Error("hlsearch did not bring the marks back")
	}
}

func TestWrapScanOptionDecidesWhetherNAndNWrapInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted) // rows 2 and 4; cursor on 2
	harness.setOptions(options.Values{options.WrapScan: false})

	harness.press("N")
	if got, text := harness.view.selection(), harness.lastInfo(); got != 2 || text != "no more matches above" {
		test.Errorf("N on the first match: row %d, status %q", got, text)
	}
	harness.press("n")
	harness.press("n")
	if got, text := harness.view.selection(), harness.lastInfo(); got != 4 || text != "no more matches below" {
		test.Errorf("n on the last match: row %d, status %q", got, text)
	}
	harness.setOptions(options.Values{options.WrapScan: true})
	harness.press("n")
	if got := harness.view.selection(); got != 2 {
		test.Errorf("n with wrapscan: %d, want it wrapped to 2", got)
	}
}
