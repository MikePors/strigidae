package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

func searchModel(test testing.TB) *Model {
	test.Helper()
	model := threadModel(test, 100, 80)
	model.SetHighlightSearch(true)
	return model
}

func typeSearch(model *Model, text string, state userinterface.PromptState) {
	_ = model.SearchPrompt(userinterface.PromptResultMessage{Prefix: search.Prefix, Text: text, State: state})
}

// startSearch types text on the prompt and keeps it with enter.
func startSearch(model *Model, text string) {
	_ = model.StartSearch()
	typeSearch(model, text, userinterface.PromptChanged)
	typeSearch(model, text, userinterface.PromptSubmitted)
}

// markedRows returns the rows drawn with the match marker, as plain text.
func markedRows(model *Model) []string {
	var marked []string
	for _, line := range strings.Split(screen(model), "\n") {
		if strings.HasPrefix(line, search.Glyph) {
			marked = append(marked, strings.TrimSpace(line))
		}
	}
	return marked
}

func TestSearchMatchesADiffLine(test *testing.T) {
	model := searchModel(test)
	model.GotoBottom()
	startSearch(model, "HELLO")
	row := model.CursorRow()
	if got := model.searchItems()[row].Text; !strings.Contains(got, `"hello"`) {
		test.Errorf("cursor row is %q", got)
	}
	if model.rows[row].threadRow != nil {
		test.Error("matched a thread row")
	}
	model.GotoTop()
	if got := markedRows(model); len(got) != 1 || !strings.Contains(got[0], `Fprintln(w, "hello")`) {
		test.Errorf("marked rows = %q", got)
	}
}

func TestSearchDoesNotLookInTheGutter(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "32") // the deleted line's old number
	model.GotoTop()
	if got := markedRows(model); len(got) != 0 {
		test.Errorf("a line number matched: %q", got)
	}
}

func TestSearchMatchesACommentRow(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "need log")
	row := model.CursorRow()
	if model.rows[row].threadRow == nil {
		test.Fatalf("cursor row %d is not a thread row", row)
	}
	if !strings.Contains(model.searchItems()[row].Text, "Do we need log here?") {
		test.Errorf("cursor row text = %q", model.searchItems()[row].Text)
	}
	model.GotoTop()
	marked := markedRows(model)
	if len(marked) != 1 || !strings.Contains(marked[0], "Do we need log here?") {
		test.Errorf("marked rows = %q", marked)
	}
}

func TestSearchJumpsToTheFirstMatchWhileTyping(test *testing.T) {
	model := searchModel(test)
	_ = model.StartSearch()
	typeSearch(model, "fprintln", userinterface.PromptChanged)
	first := model.CursorRow()
	if !strings.Contains(model.searchItems()[first].Text, `Fprintln(w, "hello")`) {
		test.Fatalf("first jump went to %q", model.searchItems()[first].Text)
	}
	typeSearch(model, "fprintln x", userinterface.PromptChanged)
	if model.CursorRow() != 0 {
		test.Errorf("with no match the cursor is on %d, want back on 0", model.CursorRow())
	}
	typeSearch(model, "fprintln", userinterface.PromptCancelled)
	if model.CursorRow() != 0 || model.search.Active() {
		test.Errorf("esc left cursor %d, active %v", model.CursorRow(), model.search.Active())
	}
}

func TestSearchNextPreviousAndCounts(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "fprintln")
	var rows []int
	for i, item := range model.searchItems() {
		if strings.Contains(item.Text, "Fprintln") {
			rows = append(rows, i)
		}
	}
	if len(rows) != 2 {
		test.Fatalf("matching rows = %v", rows)
	}
	step := func(forward bool, count int, wrapScan bool) {
		_ = model.SearchStep(keys.Message{Count: count}, forward, wrapScan)
	}
	if model.CursorRow() != rows[0] {
		test.Fatalf("cursor = %d, want %d", model.CursorRow(), rows[0])
	}
	step(true, 0, true)
	if model.CursorRow() != rows[1] {
		test.Errorf("n: %d, want %d", model.CursorRow(), rows[1])
	}
	step(true, 0, true)
	if model.CursorRow() != rows[0] {
		test.Errorf("n wraps: %d, want %d", model.CursorRow(), rows[0])
	}
	step(false, 0, true)
	if model.CursorRow() != rows[1] {
		test.Errorf("N wraps: %d, want %d", model.CursorRow(), rows[1])
	}
	step(true, 3, true) // b, a, b, a
	if model.CursorRow() != rows[0] {
		test.Errorf("3n: %d, want %d", model.CursorRow(), rows[0])
	}
	// Without wrapscan the search stops at the ends.
	step(false, 0, false)
	if model.CursorRow() != rows[0] {
		test.Errorf("N without wrapscan moved to %d", model.CursorRow())
	}
	step(true, 5, false)
	if model.CursorRow() != rows[0] {
		test.Errorf("5n without wrapscan moved to %d", model.CursorRow())
	}
	step(true, 0, false)
	step(true, 0, false)
	if model.CursorRow() != rows[1] {
		test.Errorf("n at the last match without wrapscan: %d, want %d", model.CursorRow(), rows[1])
	}
}

func TestSearchClear(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "fprintln")
	model.GotoTop()
	if len(markedRows(model)) == 0 {
		test.Fatal("no marks before clearing")
	}
	model.ClearSearch()
	if model.search.Active() || len(markedRows(model)) != 0 {
		test.Errorf("after esc: active %v, marks %q", model.search.Active(), markedRows(model))
	}
}

func TestSearchMarksOnlyWithHighlightSearch(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "fprintln")
	model.GotoTop()
	model.SetHighlightSearch(false)
	if got := markedRows(model); len(got) != 0 {
		test.Errorf("marked with hlsearch off: %q", got)
	}
}

func TestSearchFoldedAndHiddenThreads(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "yes, for")
	if model.rows[model.CursorRow()].threadRow == nil {
		test.Fatal("an open thread's reply did not match")
	}
	model.SetThreadExpanded("open", false)
	model.GotoTop()
	if len(markedRows(model)) != 0 {
		test.Error("a folded thread matched on a row that is not drawn")
	}
	model.SetThreadExpanded("open", true)
	if len(markedRows(model)) != 1 {
		test.Errorf("unfolding did not bring the mark back: %q", markedRows(model))
	}

	model.SetThreadExpanded("resolved", true)
	startSearch(model, "blank line please")
	model.GotoTop()
	if len(markedRows(model)) != 1 {
		test.Fatalf("resolved thread not found: %q", markedRows(model))
	}
	model.SetHideResolved(true)
	if got := markedRows(model); len(got) != 0 {
		test.Errorf("a hidden resolved thread matched: %q", got)
	}
}

func TestSearchRowsRebuiltKeepTheQuery(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "fprintln")
	model.GotoTop()
	before := len(markedRows(model))
	model.SetThreadExpanded("open", false) // shifts the rows below the thread
	model.SetSize(60, 80)
	model.GotoTop()
	if model.search.Query() != "fprintln" {
		test.Fatalf("query = %q", model.search.Query())
	}
	marked := markedRows(model)
	if before != 2 || len(marked) != before {
		test.Errorf("marked rows = %q, want %d", marked, before)
	}
	for _, line := range marked {
		if !strings.Contains(line, "Fprintln") {
			test.Errorf("mark on the wrong row: %q", line)
		}
	}
}

func TestSearchWrappedLineMatchesOnItsRow(test *testing.T) {
	model := wrapModel(test, 10)
	model.SetHighlightSearch(true)
	startSearch(model, "xyz") // in the tail of the long line, on a later screen row
	model.GotoTop()
	lines := strings.Split(ansi.Strip(model.Render(21, 10)), "\n")
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, search.Glyph) {
			count++
		}
	}
	if count != 1 {
		test.Errorf("marks = %d, want 1 (on the first screen row of the line)\n%s", count, strings.Join(lines, "\n"))
	}
	if !strings.HasPrefix(lines[2], search.Glyph) {
		test.Errorf("the mark is not on the line's first row: %q", lines[2])
	}
}

func TestSearchQuerySurvivesRefreshButNotAnotherFile(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "fprintln")
	model.SetFile(fixture(test, "server.go")) // a re-parse of the same path
	if model.search.Query() != "fprintln" {
		test.Errorf("a refresh dropped the query: %q", model.search.Query())
	}
	model.SetMessage("Loading diff…")
	model.SetFile(fixture(test, "server.go"))
	if model.search.Query() != "fprintln" {
		test.Errorf("a reload dropped the query: %q", model.search.Query())
	}
	model.SetFile(fixture(test, "pkg/new.go"))
	if model.search.Active() {
		test.Error("another file kept the search")
	}
}

func TestSearchMarkerGolden(test *testing.T) {
	model := searchModel(test)
	startSearch(model, "log")
	model.GotoTop()
	golden.Assert(test, "search_marked", screen(model), false)
}
