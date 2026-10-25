package pullrequestlist

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

func infoTexts(command tea.Cmd) []string {
	if command == nil {
		return nil
	}
	switch message := command().(type) {
	case userinterface.InfoMessage:
		return []string{message.Text}
	case tea.BatchMsg:
		var texts []string
		for _, inner := range message {
			texts = append(texts, infoTexts(inner)...)
		}
		return texts
	}
	return nil
}

func searchedView(test *testing.T) *View {
	test.Helper()
	view := searchView(test)
	press(view, "/")
	typeSearch(view, "fix")
	finishSearch(view, "fix", userinterface.PromptSubmitted) // cursor on row 1; matches are 1, 3 and 5
	return view
}

func TestHighlightSearchOptionHidesAndRestoresTheMarks(test *testing.T) {
	view := searchedView(test)
	shown := func() bool { return strings.Contains(ansi.Strip(view.Render(80, 12)), search.Glyph) }
	if !shown() {
		test.Fatal("marks are not drawn by default")
	}
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.HighlightSearch: false}})
	if shown() {
		test.Error("nohlsearch still draws the marks")
	}
	press(view, "n")
	if got := view.window.Cursor(); got != 3 {
		test.Errorf("n with nohlsearch: cursor = %d, want 3", got)
	}
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.HighlightSearch: true}})
	if !shown() {
		test.Error("hlsearch did not bring the marks back")
	}
}

func TestWrapScanOptionDecidesWhetherNAndNWrap(test *testing.T) {
	view := searchedView(test)
	view.Update(userinterface.OptionsMessage{Values: options.Values{options.WrapScan: false}})

	press(view, "N")
	if got := view.window.Cursor(); got != 1 {
		test.Errorf("N on the first match moved to %d", got)
	}
	if got := infoTexts(press(view, "N")); len(got) != 1 || got[0] != "no more matches above" {
		test.Errorf("N on the first match said %q", got)
	}
	press(view, "n")
	press(view, "n")
	if got := view.window.Cursor(); got != 5 {
		test.Fatalf("cursor = %d, want 5", got)
	}
	if got := infoTexts(press(view, "n")); len(got) != 1 || got[0] != "no more matches below" {
		test.Errorf("n on the last match said %q", got)
	}
	if got := view.window.Cursor(); got != 5 {
		test.Errorf("n on the last match moved to %d", got)
	}

	view.Update(userinterface.OptionsMessage{Values: options.Values{options.WrapScan: true}})
	press(view, "n")
	if got := view.window.Cursor(); got != 1 {
		test.Errorf("n with wrapscan: cursor = %d, want it wrapped to 1", got)
	}
}
