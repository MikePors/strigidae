package diffview

import (
	"fmt"
	"strings"
	"testing"

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

// gapModelWithText is gapModel with the file's text given: count lines,
// "line 1" to "line <count>".
func gapModelWithText(test *testing.T, patch string, count int) *Model {
	test.Helper()
	model := gapModel(test, patch)
	if err := model.SetFileText(numberedText(count)); err != nil {
		test.Fatal(err)
	}
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

// goToDiffLine puts the cursor on the row of the diff line with this text.
func goToDiffLine(test *testing.T, model *Model, text string) {
	test.Helper()
	for i, row := range model.rows {
		if row.isLine() && row.line.Text == text {
			model.window.GotoRow(i)
			return
		}
	}
	test.Fatalf("no diff line %q", text)
}
