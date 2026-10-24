package diffview

import (
	"testing"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func syntheticFile(status diff.Status, hunks ...diff.Hunk) *diff.File {
	return &diff.File{OldPath: "a.go", NewPath: "a.go", Status: status, Hunks: hunks}
}

func contextLines(first, last int) diff.Hunk {
	hunk := diff.Hunk{OldStart: first, NewStart: first}
	for number := first; number <= last; number++ {
		hunk.Lines = append(hunk.Lines, diff.Line{Kind: diff.Context, Old: number, New: number, Text: "x"})
	}
	return hunk
}

func TestGotoLine(test *testing.T) {
	tests := []struct {
		name string
		file *diff.File
		line int
		want int // the line number the cursor lands on
	}{
		{"exact hit", syntheticFile(diff.Modified, contextLines(1, 10), contextLines(31, 39)), 33, 33},
		{"line between hunks goes to the closer one below", syntheticFile(diff.Modified, contextLines(1, 10), contextLines(31, 39)), 14, 10},
		{"line between hunks goes to the closer one above", syntheticFile(diff.Modified, contextLines(1, 10), contextLines(31, 39)), 28, 31},
		{"a tie prefers the next line", syntheticFile(diff.Modified, contextLines(1, 10), contextLines(12, 20)), 11, 12},
		{"line past the end goes to the last line", syntheticFile(diff.Modified, contextLines(1, 10), contextLines(31, 39)), 500, 39},
		{"line zero goes to the first line", syntheticFile(diff.Modified, contextLines(5, 10)), 0, 5},
		{
			"deleted file uses old-side numbers",
			syntheticFile(diff.Deleted, diff.Hunk{OldStart: 1, Lines: []diff.Line{
				{Kind: diff.Delete, Old: 1, Text: "a"}, {Kind: diff.Delete, Old: 2, Text: "b"}, {Kind: diff.Delete, Old: 3, Text: "c"},
			}}),
			2, 2,
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := New(theme.New(true))
			model.SetFile(testCase.file)
			model.SetSize(80, 10)
			model.GotoLine(testCase.line)
			cursorLine, ok := model.Cursor()
			if !ok {
				test.Fatal("cursor is not on a diff line")
			}
			got := cursorLine.New
			if testCase.file.Status == diff.Deleted {
				got = cursorLine.Old
			}
			if got != testCase.want {
				test.Errorf("GotoLine(%d) landed on line %d, want %d", testCase.line, got, testCase.want)
			}
		})
	}
}

func TestGotoLineIgnoresDeletedLinesInChangedFiles(test *testing.T) {
	hunk := diff.Hunk{OldStart: 1, NewStart: 1, Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Delete, Old: 2, Text: "gone"},
		{Kind: diff.Add, New: 2, Text: "new"},
	}}
	model := New(theme.New(true))
	model.SetFile(syntheticFile(diff.Modified, hunk))
	model.SetSize(80, 10)
	model.GotoLine(2)
	cursorLine, ok := model.Cursor()
	if !ok || cursorLine.Kind != diff.Add || cursorLine.New != 2 {
		test.Errorf("cursor = %+v, want the added line 2", cursorLine)
	}
}

func TestGotoLineWithoutLinesDoesNothing(test *testing.T) {
	model := New(theme.New(true))
	model.SetFile(syntheticFile(diff.Modified))
	model.SetSize(80, 10)
	model.GotoLine(5)
	if model.CursorRow() != 0 {
		test.Errorf("cursor row = %d, want 0", model.CursorRow())
	}
}
