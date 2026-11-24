package diffview

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
)

func TestExpandGapNeedsTheFileText(test *testing.T) {
	model := gapModel(test, twoHunks)
	if model.HasFileText() || !model.OnGap() {
		test.Fatal("setup")
	}
	before := len(model.rows)
	model.ExpandGap()
	if len(model.rows) != before {
		test.Error("a gap opened without the file's text")
	}
}

func TestExpandGapDoesNothingOffAGap(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	model.CursorDown(2) // a diff line
	before := len(model.rows)
	model.ExpandGap()
	if len(model.rows) != before {
		test.Error("a gap opened with the cursor on a diff line")
	}
}

func TestExpandingTheTopGapShowsTheLinesNearestTheHunk(test *testing.T) {
	model := gapModelWithText(test, "@@ -40,2 +40,2 @@\n a\n-b\n+B\n", 60)
	wantGaps(test, model, "39 hidden lines", "19 hidden lines")
	model.ExpandGap()
	wantGaps(test, model, "19 hidden lines", "19 hidden lines")
	shown := shownLines(model)
	if len(shown) != gapChunk || shown[0].New != 20 || shown[len(shown)-1].New != 39 {
		test.Fatalf("shown lines = %+v, want 20..39", shown)
	}
	if !model.OnGap() {
		test.Error("the cursor left the gap row")
	}
	model.ExpandGap()
	wantGaps(test, model, "19 hidden lines")
	shown = shownLines(model)
	if len(shown) != 39 || shown[0].New != 1 {
		test.Fatalf("shown %d lines from %d, want 39 from 1", len(shown), shown[0].New)
	}
	// The gap is gone, so the cursor is on the first line shown, under the
	// hunk's header.
	if cursor := model.window.Cursor(); !model.rows[cursor].revealed || model.rows[cursor].line.New != 1 {
		test.Errorf("cursor row %d is %+v, want line 1", cursor, model.rows[cursor])
	}
}

func TestExpandingBetweenHunksShowsALineAtATimeAfterTheFirst(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	goToGap(test, model, 1)
	model.ExpandGap()
	wantGaps(test, model, "19 hidden lines", "6 hidden lines", "9 hidden lines")
	if !model.OnGap() || model.rows[model.window.Cursor()].gap.index != 1 {
		test.Fatal("the cursor left the gap row")
	}
	shown := shownLines(model)
	if len(shown) != gapChunk {
		test.Fatalf("%d lines shown, want %d", len(shown), gapChunk)
	}
	if want := (diff.Line{Kind: diff.Context, Old: 24, New: 23, Text: "line 23"}); shown[0] != want {
		test.Errorf("first line shown = %+v, want %+v", shown[0], want)
	}
	if want := (diff.Line{Kind: diff.Context, Old: 43, New: 42, Text: "line 42"}); shown[len(shown)-1] != want {
		test.Errorf("last line shown = %+v, want %+v", shown[len(shown)-1], want)
	}

	model.ExpandGap() // six are left: they all open
	wantGaps(test, model, "19 hidden lines", "9 hidden lines")
	if row := model.rows[model.window.Cursor()]; !row.revealed || row.line.New != 43 {
		test.Errorf("cursor is on %+v, want the first of the new lines (43)", row)
	}
	if shown = shownLines(model); len(shown) != 26 || shown[25].New != 48 {
		test.Errorf("%d lines shown ending at %d, want 26 ending at 48", len(shown), shown[len(shown)-1].New)
	}
}

func TestShownLinesTakeNoComment(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	model.GotoBottom()
	model.ExpandGap() // the nine lines below the last hunk; the cursor is on the first
	if _, ok := model.Cursor(); ok {
		test.Error("a line outside the diff reports a diff cursor")
	}
	context := model.Context()
	if context.HasLines || !context.OnExpandedLine {
		test.Errorf("context on a shown line: HasLines %v, OnExpandedLine %v", context.HasLines, context.OnExpandedLine)
	}
	if model.VisualStart() {
		test.Error("a selection started on a line outside the diff")
	}
	model.GotoTop()
	if model.Context().OnExpandedLine {
		test.Error("a gap row counts as a shown line")
	}
}

func TestGapsStartOverWhenTheFileIsRefreshed(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	model.ExpandGap()
	model.SetFile(parseFile(test, twoHunks)) // a refresh: the same file, a new value
	if model.HasFileText() || len(shownLines(model)) != 0 {
		test.Error("the text of the old diff was kept")
	}
	wantGaps(test, model, "19 hidden lines", "26 hidden lines", "more lines")
}

func TestSelectionSurvivesTheTextArriving(test *testing.T) {
	model := gapModel(test, twoHunks)
	goToDiffLine(test, model, "c")
	if !model.VisualStart() {
		test.Fatal("setup: no selection")
	}
	before, ok := model.selection()
	if !ok {
		test.Fatal("setup: no range")
	}
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	after, ok := model.selection()
	if !model.VisualActive() || !ok {
		test.Fatal("the text arriving ended the selection")
	}
	if difference := cmp.Diff(before, after); difference != "" {
		test.Errorf("the selection moved (-before +after):\n%s", difference)
	}
}

func TestExpandingAGapEndsTheSelectionOnIt(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	goToDiffLine(test, model, "c")
	model.VisualStart()
	goToGap(test, model, 1) // the cursor can leave the lines by other keys than the selection's
	model.ExpandGap()
	if model.VisualActive() {
		test.Error("a selection outlived the gap the cursor was on")
	}
}

func TestRefreshKeepsTheCursorOnItsGap(test *testing.T) {
	for _, index := range []int{0, 1, 2} {
		model := gapModel(test, twoHunks)
		goToGap(test, model, index)
		model.SetFile(parseFile(test, twoHunks))
		if got := model.GapIndex(); got != index {
			test.Errorf("after a refresh the cursor is on gap %d, want %d (row %d)", got, index, model.window.Cursor())
		}
	}
}

func TestGotoLineReachesShownLines(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	goToGap(test, model, 0)
	model.ExpandGap() // lines 1 to 19 are shown
	model.GotoLine(10)
	if row := model.rows[model.window.Cursor()]; !row.revealed || row.line.New != 10 {
		test.Errorf("cursor is on %+v, want shown line 10", row)
	}
}

func TestGapNextToAHiddenHunkStillShowsItsLines(test *testing.T) {
	// The hidden hunk's own lines stay out; the unchanged lines beside it show.
	model := gapModel(test, twoHunks)
	model.SetHunkHidden(func(file *diff.File, hunk int) bool { return file == model.file && hunk == 0 })
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}

	goToGap(test, model, 1)
	model.ExpandGap() // twenty of the 26 between the hunks
	if shown := shownLines(model); len(shown) != gapChunk || shown[0].New != 23 {
		test.Fatalf("%d lines shown from %+v, want %d from line 23", len(shown), shown, gapChunk)
	}
	if model.GapIndex() != 1 {
		test.Errorf("the cursor left the gap that has lines left (row %d)", model.window.Cursor())
	}

	goToGap(test, model, 0)
	model.ExpandGap() // the 19 above the hidden hunk, all of them
	cursor := model.window.Cursor()
	if row := model.rows[cursor]; !row.revealed || row.line.New != 1 || cursor != model.placeholderRow(0)+1 {
		test.Errorf("cursor row %d is %+v, want line 1 under the hidden hunk's placeholder", cursor, row)
	}
	if shown := shownLines(model); len(shown) != 19+gapChunk {
		test.Errorf("%d lines shown, want %d", len(shown), 19+gapChunk)
	}
}

func TestShownLinesAreInNoHunk(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	goToGap(test, model, 0)
	model.ExpandGap()
	if context := model.Context(); context.InHunk {
		test.Errorf("a line the diff leaves out is in hunk %d", context.Hunk)
	}
}

func TestRefreshFindsItsGapWhenAHunkComesAbove(test *testing.T) {
	model := gapModel(test, twoHunks)
	goToGap(test, model, 1) // the gap above the hunk at line 50
	const withNewHunk = "@@ -2,3 +2,3 @@\n x\n-y\n+Y\n z\n" + twoHunks
	model.SetFile(parseFile(test, withNewHunk))
	gap := model.rows[model.window.Cursor()].gap
	if gap == nil || gap.index != 2 {
		test.Errorf("cursor is on %+v, want the gap above the same hunk (gap 2)", model.rows[model.window.Cursor()])
	}
}
