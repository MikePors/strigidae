package diffview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// twoHunks leaves new lines 1..19 above the first hunk (new 20..22), 23..48
// between the hunks and 52.. below the second (new 49..51). The first hunk
// deletes a line, so from line 23 on a line's old number is one more than its
// new number.
const twoHunks = "@@ -20,4 +20,3 @@\n a\n-b\n c\n d\n@@ -50,3 +49,3 @@\n e\n-f\n+F\n g\n"

// numberedText is a file of count lines, "line 1" to "line <count>".
func numberedText(count int) string {
	var builder strings.Builder
	for number := 1; number <= count; number++ {
		fmt.Fprintf(&builder, "line %d\n", number)
	}
	return builder.String()
}

// gapModel shows patch as a modified file, tall enough to show every row.
func gapModel(test *testing.T, patch string) *Model {
	test.Helper()
	model := New(theme.New(true))
	model.SetFile(parseFile(test, patch))
	model.SetSize(60, 60)
	model.GotoTop() // a file opens on its first header; the tests start on the row above
	return model
}

// gapTexts returns what every gap row says, in order.
func gapTexts(model *Model) []string {
	var texts []string
	for _, row := range model.rows {
		if row.gap != nil {
			texts = append(texts, hiddenText(row.gap.hidden))
		}
	}
	return texts
}

// goToGap puts the cursor on the row of gap index.
func goToGap(test *testing.T, model *Model, index int) {
	test.Helper()
	row := model.gapRowIndex(index)
	if row < 0 {
		test.Fatalf("no row for gap %d in %v", index, gapTexts(model))
	}
	model.window.GotoRow(row)
}

// shownLines returns the lines shown by expanding gaps, in order.
func shownLines(model *Model) []diff.Line {
	var shown []diff.Line
	for _, row := range model.rows {
		if row.revealed {
			shown = append(shown, *row.line)
		}
	}
	return shown
}

func wantGaps(test *testing.T, model *Model, want ...string) {
	test.Helper()
	if difference := cmp.Diff(want, gapTexts(model)); difference != "" {
		test.Errorf("gap rows (-want +got):\n%s", difference)
	}
}

func TestGapRowsStandForUnchangedLines(test *testing.T) {
	model := gapModel(test, twoHunks)
	wantGaps(test, model, "19 hidden lines", "26 hidden lines", "more lines")
	lines := strings.Split(screen(model), "\n")
	if got, want := strings.TrimRight(lines[0], " "), ">"+strings.Repeat(" ", model.lineNumbersWidth())+"⋯ 19 hidden lines"; got != want {
		test.Errorf("first row = %q, want %q", got, want)
	}
	if got := strings.TrimRight(lines[1], " "); got != " @@ -20,4 +20,3 @@" {
		test.Errorf("second row = %q, want the first hunk's header", got)
	}
}

func TestFileOpensOnItsFirstHunkHeader(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(parseFile(test, twoHunks))
	model.SetSize(60, 60)
	if model.OnGap() || model.window.Cursor() != 1 || model.rows[1].gap != nil {
		test.Errorf("cursor on row %d, want the header under the gap", model.window.Cursor())
	}
	model = New(theme.New(true))
	model.SetFile(parseFile(test, "@@ -1,2 +1,2 @@\n a\n-b\n+B\n"))
	model.SetSize(60, 60)
	if model.window.Cursor() != 0 {
		test.Errorf("cursor on row %d, want the header at the top", model.window.Cursor())
	}
}

func TestOneLineGapIsSingular(test *testing.T) {
	model := gapModel(test, "@@ -2,2 +2,2 @@\n a\n-b\n+B\n")
	wantGaps(test, model, "1 hidden line", "more lines")
}

func TestHunksThatTouchLeaveNoGap(test *testing.T) {
	model := gapModel(test, "@@ -1,2 +1,2 @@\n a\n-b\n+B\n@@ -3,2 +3,2 @@\n c\n-d\n+D\n")
	wantGaps(test, model, "more lines")
}

func TestGapRowsOnlyForFilesWithBothSides(test *testing.T) {
	for _, status := range []diff.Status{diff.Added, diff.Deleted} {
		file := parseFile(test, twoHunks)
		file.Status = status
		model := New(theme.New(true))
		model.SetFile(file)
		model.SetSize(60, 10)
		if texts := gapTexts(model); len(texts) != 0 {
			test.Errorf("%s file has gap rows %v", status, texts)
		}
		if err := model.SetFileText(numberedText(40)); err != nil || model.HasFileText() {
			test.Errorf("%s file kept text: %v", status, err)
		}
	}
}

func TestFileTextCountsTheLinesBelowTheLastHunk(test *testing.T) {
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	wantGaps(test, model, "19 hidden lines", "26 hidden lines", "9 hidden lines")
}

func TestGapBelowTheLastHunkGoesWhenTheFileEndsThere(test *testing.T) {
	model := gapModel(test, twoHunks)
	model.GotoBottom()
	if !model.OnGap() {
		test.Fatal("setup: the cursor is not on the last gap")
	}
	if err := model.SetFileText(numberedText(51)); err != nil { // the second hunk's last line is 51
		test.Fatal(err)
	}
	wantGaps(test, model, "19 hidden lines", "26 hidden lines")
	if model.window.Cursor() >= len(model.rows) {
		test.Errorf("cursor %d is past the %d rows", model.window.Cursor(), len(model.rows))
	}
}

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
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	model.CursorDown(2) // a diff line
	before := len(model.rows)
	model.ExpandGap()
	if len(model.rows) != before {
		test.Error("a gap opened with the cursor on a diff line")
	}
}

func TestExpandingTheTopGapShowsTheLinesNearestTheHunk(test *testing.T) {
	model := gapModel(test, "@@ -40,2 +40,2 @@\n a\n-b\n+B\n")
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
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
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
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
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
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

func TestGapRowsAreNeitherALineNorInAHunk(test *testing.T) {
	model := gapModel(test, twoHunks)
	if model.Context().InHunk {
		test.Error("a gap row is in a hunk")
	}
	if _, ok := model.Cursor(); ok {
		test.Error("a gap row is a diff line")
	}
}

func TestFileTextRefusesBinaryAndHugeText(test *testing.T) {
	model := gapModel(test, twoHunks)
	if err := model.SetFileText("a\x00b\n"); err == nil || model.HasFileText() {
		test.Errorf("binary text accepted: %v", err)
	}
	if err := model.SetFileText(strings.Repeat("x", MaximumFileSize+1)); err == nil || model.HasFileText() {
		test.Errorf("huge text accepted: %v", err)
	}
}

func TestFileTextWithWindowsLineEndings(test *testing.T) {
	model := gapModel(test, "@@ -3,2 +3,2 @@\n a\n-b\n+B\n")
	if err := model.SetFileText("one\r\ntwo\r\nthree\r\nfour\r\n"); err != nil {
		test.Fatal(err)
	}
	goToGap(test, model, 0)
	model.ExpandGap()
	if shown := shownLines(model); len(shown) != 2 || shown[0].Text != "one" || shown[1].Text != "two" {
		test.Errorf("shown lines = %+v", shown)
	}
}

func TestFileTextShorterThanTheDiffShowsWhatItHas(test *testing.T) {
	model := gapModel(test, twoHunks) // the text should reach line 48 at least
	if err := model.SetFileText(numberedText(30)); err != nil {
		test.Fatal(err)
	}
	goToGap(test, model, 1)
	for range 3 {
		model.ExpandGap()
	}
	if shown := shownLines(model); len(shown) == 0 || shown[len(shown)-1].New > 30 {
		test.Errorf("shown lines run past the text: %+v", shown)
	}
}

func TestGapsStartOverWhenTheFileIsRefreshed(test *testing.T) {
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	model.ExpandGap()
	model.SetFile(parseFile(test, twoHunks)) // a refresh: the same file, a new value
	if model.HasFileText() || len(shownLines(model)) != 0 {
		test.Error("the text of the old diff was kept")
	}
	wantGaps(test, model, "19 hidden lines", "26 hidden lines", "more lines")
}

func TestShownLinesAreDrawnAndSearched(test *testing.T) {
	model := gapModel(test, twoHunks)
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	model.ExpandGap() // the 19 lines above the first hunk
	if !strings.Contains(screen(model), "line 19") {
		test.Errorf("the line nearest the first hunk is not drawn:\n%s", screen(model))
	}
	found := false
	for _, item := range model.searchItems() {
		found = found || item.Text == "line 19"
	}
	if !found {
		test.Error("a shown line is not searched")
	}
}

func TestGapRowFillsTheWidthAtEveryWidth(test *testing.T) {
	for _, width := range []int{12, 20, 40} {
		model := gapModel(test, twoHunks)
		model.SetSize(width, 10)
		for i, line := range strings.Split(screen(model), "\n") {
			if got := ansi.StringWidth(line); got != width {
				test.Errorf("width %d: row %d is %d cells wide", width, i, got)
			}
		}
	}
}
