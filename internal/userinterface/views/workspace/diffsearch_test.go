package workspace

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
)

// diffMarked returns the diff pane's rows drawn with the match marker.
func (harness *harness) diffMarked() []string {
	harness.test.Helper()
	var marked []string
	for _, line := range strings.Split(ansi.Strip(harness.view.filePane.Render(100, 30)), "\n") {
		if strings.HasPrefix(line, search.Glyph) {
			marked = append(marked, line)
		}
	}
	return marked
}

// cursorLine names the diff line under the cursor in the diff pane.
func (harness *harness) cursorLine() string {
	harness.test.Helper()
	context := harness.view.filePane.Context()
	if !context.HasLines {
		return "none"
	}
	return fmt.Sprintf("%d%s", context.Lines.Start, context.Lines.Side)
}

// diffSearching opens client.go, focuses the diff pane and searches it.
func diffSearching(test *testing.T, text string) *harness {
	test.Helper()
	harness := loaded(test, 100, 30)
	harness.press("j", "l")
	harness.press("/")
	harness.searchFor(text, userinterface.PromptChanged)
	harness.searchFor(text, userinterface.PromptSubmitted)
	return harness
}

func TestSlashInTheDiffPaneAsksForThePromptAndSearchesTheDiff(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j", "l", "/")
	if prefix, ok := harness.view.PromptRequest(); !ok || prefix != "/" {
		test.Fatalf("after / in the diff pane the request is %q, %v", prefix, ok)
	}
	harness.searchFor("c.last", userinterface.PromptChanged)
	harness.searchFor("c.last", userinterface.PromptSubmitted)
	if harness.view.search.Active() {
		test.Error("the Files pane search was set by a diff search")
	}
	if got := harness.view.selectedPath(); got != "internal/sync/client.go" {
		test.Errorf("the selection moved to %q", got)
	}
	if marked := harness.diffMarked(); len(marked) != 0 {
		// The cursor is on the match, which shows the cursor instead.
		test.Errorf("marked = %q", marked)
	}
	if got := harness.cursorLine(); got != "46RIGHT" { // c.last = time.Now()
		test.Errorf("the cursor is on line %s, want 46RIGHT", got)
	}
}

func TestDiffSearchNextPreviousAndEsc(test *testing.T) {
	harness := diffSearching(test, "ctx")
	cursor := harness.cursorLine
	first := cursor()
	harness.press("n")
	second := cursor()
	if second == first {
		test.Errorf("n stayed on %s", second)
	}
	harness.press("N")
	if cursor() != first {
		test.Errorf("N went to %q, want %q", cursor(), first)
	}
	harness.press("esc")
	if len(harness.diffMarked()) != 0 {
		test.Error("esc left marks")
	}
	harness.press("n")
	if cursor() != first {
		test.Error("n after esc moved the cursor")
	}
}

func TestDiffAndFilesSearchesKeepTheirOwnQueries(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("retry") // Files pane: retry.go
	harness.searchFor("retry", userinterface.PromptSubmitted)
	harness.press("l", "/")
	harness.searchFor("ctx", userinterface.PromptChanged)
	harness.searchFor("ctx", userinterface.PromptSubmitted)
	if got := harness.view.search.Query(); got != "retry" {
		test.Errorf("Files query = %q", got)
	}
	harness.press("h", "n")
	if got := harness.view.selection(); got != 4 {
		test.Errorf("n in the Files pane went to entry %d, want 4", got)
	}
	harness.press("esc") // clears the Files search only
	if harness.view.search.Active() {
		test.Error("esc kept the Files search")
	}
}

func TestChangingFileClearsTheDiffSearch(test *testing.T) {
	harness := diffSearching(test, "ctx")
	harness.press("g g")
	if len(harness.diffMarked()) == 0 {
		test.Fatal("no marks before changing file")
	}
	harness.press("h", "l") // only the focus changed: the search stays
	if len(harness.diffMarked()) == 0 {
		test.Error("the search was dropped without a file change")
	}
	harness.press("h")
	harness.press("j") // retry.go
	harness.press("k") // client.go again
	harness.press("l")
	if len(harness.diffMarked()) != 0 {
		test.Error("the diff search survived a change of file")
	}
}
