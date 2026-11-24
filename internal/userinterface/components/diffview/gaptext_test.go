package diffview

import (
	"strings"
	"testing"
)

func TestFileTextCountsTheLinesBelowTheLastHunk(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
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

func TestSecondTextChangesNothing(test *testing.T) {
	model := gapModelWithText(test, twoHunks, 60)
	goToGap(test, model, 0)
	model.ExpandGap()
	cursor := model.window.Cursor()
	if err := model.SetFileText(numberedText(60)); err != nil {
		test.Fatal(err)
	}
	if model.window.Cursor() != cursor || !model.rows[cursor].revealed {
		test.Errorf("a repeated text moved the cursor from row %d to %d", cursor, model.window.Cursor())
	}
}
