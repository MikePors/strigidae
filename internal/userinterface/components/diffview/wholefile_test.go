package diffview

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// insideCommentFile is a Go file whose one hunk starts inside a block comment
// opened above it, with the whole text of each side.
func insideCommentFile() (file *diff.File, oldText, newText string) {
	oldText = "package x\n\n/* opened above\nstill comment\nclosed */\nvar a = 1\n"
	newText = "package x\n\n/* opened above\nstill comment\nclosed */\nvar b = 2\n"
	file = &diff.File{
		OldPath: "x.go", NewPath: "x.go", Status: diff.Modified,
		OldObjectIdentifier: "1111111", NewObjectIdentifier: "2222222",
		Hunks: []diff.Hunk{{
			OldStart: 4, OldLines: 3, NewStart: 4, NewLines: 3,
			Lines: []diff.Line{
				{Kind: diff.Context, Old: 4, New: 4, Text: "still comment"},
				{Kind: diff.Context, Old: 5, New: 5, Text: "closed */"},
				{Kind: diff.Delete, Old: 6, Text: "var a = 1"},
				{Kind: diff.Add, New: 6, Text: "var b = 2"},
			},
		}},
	}
	return file, oldText, newText
}

// codeRowSpans returns the spans drawn for each code row of model.
func codeRowSpans(model *Model) [][]Span {
	var spans [][]Span
	for i, row := range model.rows {
		if row.line != nil {
			spans = append(spans, model.codeSpans(i))
		}
	}
	return spans
}

func TestFileSpansColourAHunkStartingInsideAComment(test *testing.T) {
	file, oldText, newText := insideCommentFile()
	model := New(theme.New(true))
	model.SetFile(file)
	model.SetSize(80, 20)

	before := codeRowSpans(model)
	if allIn(before[0], chroma.Comment) {
		test.Fatal("per-hunk lexing already knows the comment; the test proves nothing")
	}

	oldSpans, err := HighlightFile("x.go", []byte(oldText))
	if err != nil {
		test.Fatal(err)
	}
	newSpans, err := HighlightFile("x.go", []byte(newText))
	if err != nil {
		test.Fatal(err)
	}
	model.SetFileSpans(github.SideLeft, "1111111", oldSpans)
	model.SetFileSpans(github.SideRight, "2222222", newSpans)

	after := codeRowSpans(model)
	for i := range 2 {
		if !allIn(after[i], chroma.Comment) {
			test.Errorf("row %d = %v, want a comment", i, after[i])
		}
	}
	for i := 2; i < 4; i++ {
		if anyIn(after[i], chroma.Comment) {
			test.Errorf("code row %d = %v, want no comment", i, after[i])
		}
	}
}

func TestFileSpansAreIgnoredForAnotherFile(test *testing.T) {
	file, _, newText := insideCommentFile()
	newSpans, err := HighlightFile("x.go", []byte(newText))
	if err != nil {
		test.Fatal(err)
	}
	tests := []struct {
		name       string
		side       github.DiffSide
		identifier string
	}{
		{"other new identifier", github.SideRight, "3333333"},
		{"new identifier on the old side", github.SideLeft, "2222222"},
		{"empty identifier", github.SideRight, ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := New(theme.New(true))
			model.SetFile(file)
			model.SetFileSpans(testCase.side, testCase.identifier, newSpans)
			if allIn(codeRowSpans(model)[0], chroma.Comment) {
				test.Error("spans for another side were applied")
			}
		})
	}
}

func TestFileSpansLeaveRowsThatDoNotMatch(test *testing.T) {
	file, _, _ := insideCommentFile()
	model := New(theme.New(true))
	model.SetFile(file)
	// The fetched text says something else on every line: no row takes it.
	other := "a\nb\nc\nd\ne\nf\n"
	spans, err := HighlightFile("x.go", []byte(other))
	if err != nil {
		test.Fatal(err)
	}
	model.SetFileSpans(github.SideRight, "2222222", spans)
	for i, rowSpans := range codeRowSpans(model) {
		if got, want := joined(rowSpans), file.Hunks[0].Lines[i].Text; got != want {
			test.Errorf("row %d draws %q, want %q", i, got, want)
		}
	}
}

func TestHighlightFile(test *testing.T) {
	test.Parallel()
	tests := []struct {
		name     string
		path     string
		contents string
		wantErr  error
		// lines are the expected text of each line's spans.
		lines []string
	}{
		{name: "tabs and carriage returns as the rows show them", path: "x.go", contents: "a\tb\r\n\x1b[31m\n", lines: []string{"a   b", "·[31m", ""}},
		{name: "shebang picks the language", path: "run", contents: "#!/bin/sh\necho hi\n", lines: []string{"#!/bin/sh", "echo hi", ""}},
		{name: "no language", path: "notes", contents: "plain\n", wantErr: ErrNoLanguage},
		// These lexers hang or panic on this text; they are never picked.
		{name: "lexer that hangs", path: "q.jsonata", contents: "/* a /* b\nc\n", wantErr: ErrNoLanguage},
		{name: "lexer that panics", path: "m.yang", contents: "/* a /* b */\nc\n", wantErr: ErrNoLanguage},
		{name: "binary", path: "x.go", contents: "a\x00b", wantErr: ErrFileBinary},
		{name: "too large", path: "x.go", contents: strings.Repeat("x", MaximumFileSize+1), wantErr: ErrFileTooLarge},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			spans, err := HighlightFile(testCase.path, []byte(testCase.contents))
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			if err != nil {
				return
			}
			if len(spans) != len(testCase.lines) {
				test.Fatalf("%d lines, want %d", len(spans), len(testCase.lines))
			}
			for i, want := range testCase.lines {
				if got := joined(spans[i]); got != want {
					test.Errorf("line %d = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestFileSpansFollowEachSidesLineNumbers uses a context row whose text is
// code on its old line and at the new side's line of the same number, but a
// comment on its own new line. Only the new side, looked up by the new
// number, colours it as a comment; it is drawn after the old side arrives and
// again after the new one, so what the first draw worked out must not stick.
func TestFileSpansFollowEachSidesLineNumbers(test *testing.T) {
	oldText := "x\nx\ny\n"
	newText := "x\nx\n/* c\nx\n*/\n"
	file := &diff.File{
		OldPath: "x.go", NewPath: "x.go", Status: diff.Modified,
		OldObjectIdentifier: "1111111", NewObjectIdentifier: "2222222",
		Hunks: []diff.Hunk{{
			OldStart: 2, OldLines: 2, NewStart: 4, NewLines: 2,
			Lines: []diff.Line{
				{Kind: diff.Context, Old: 2, New: 4, Text: "x"},
				{Kind: diff.Delete, Old: 3, Text: "y"},
				{Kind: diff.Add, New: 5, Text: "*/"},
			},
		}},
	}
	model := New(theme.New(true))
	model.SetFile(file)
	model.SetSize(80, 20)
	oldSpans, err := HighlightFile("x.go", []byte(oldText))
	if err != nil {
		test.Fatal(err)
	}
	newSpans, err := HighlightFile("x.go", []byte(newText))
	if err != nil {
		test.Fatal(err)
	}

	model.SetFileSpans(github.SideLeft, "1111111", oldSpans)
	if anyIn(codeRowSpans(model)[0], chroma.Comment) {
		test.Fatal("with only the old side, the context row is a comment")
	}
	model.SetFileSpans(github.SideRight, "2222222", newSpans)
	rows := codeRowSpans(model)
	if !allIn(rows[0], chroma.Comment) {
		test.Errorf("context row = %v, want the comment of new line 4", rows[0])
	}
	if !allIn(rows[2], chroma.Comment) {
		test.Errorf("added row = %v, want the comment's end on new line 5", rows[2])
	}
}

// TestHighlightFileStopsWhenTheBudgetRunsOut lexes text that is quadratic for
// the Go lexer, by a clock that moves a second each reading, so the budget
// runs out after a few readings rather than after minutes of lexing.
func TestHighlightFileStopsWhenTheBudgetRunsOut(test *testing.T) {
	test.Parallel()
	var builder strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&builder, "value%d := %d /* note\n", i, i)
	}
	clock := time.Unix(0, 0)
	now := func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}
	spans, err := highlightFile("x.go", []byte(builder.String()), now)
	if !errors.Is(err, ErrLexTimeout) {
		test.Fatalf("err = %v, want ErrLexTimeout", err)
	}
	if spans != nil {
		test.Error("spans of part of the file were returned")
	}
}
