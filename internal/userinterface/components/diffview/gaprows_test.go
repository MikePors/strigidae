package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

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

func TestTruncatedPatchHasNoGaps(test *testing.T) {
	file := parseFile(test, twoHunks)
	file.Truncated = true // changes the patch leaves out may lie between its hunks
	model := New(theme.New(true))
	model.SetFile(file)
	model.SetSize(60, 60)
	if texts := gapTexts(model); len(texts) != 0 {
		test.Errorf("a truncated patch has gap rows %v", texts)
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

func TestShownLinesAreDrawnAndSearched(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
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

func TestSearchPlaceSurvivesAGapOpening(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	identity := func() string {
		for i, row := range model.rows {
			if row.isLine() && row.line.Text == "c" {
				return model.searchItems()[i].Identity
			}
		}
		test.Fatal("no line c")
		return ""
	}
	before := identity()
	goToGap(test, model, 0)
	model.ExpandGap()
	if after := identity(); after != before {
		test.Errorf("identity of line c changed from %q to %q", before, after)
	}
}
