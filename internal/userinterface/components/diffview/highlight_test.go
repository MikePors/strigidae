package diffview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// codeLine is one line of a test hunk.
type codeLine struct {
	kind diff.LineKind
	text string
}

// hunkFile builds a one-hunk file called path from lines.
func hunkFile(path string, lines []codeLine) *diff.File {
	hunk := diff.Hunk{OldStart: 1, NewStart: 1}
	oldNumber, newNumber := 1, 1
	for _, codeLine := range lines {
		line := diff.Line{Kind: codeLine.kind, Text: codeLine.text}
		if codeLine.kind != diff.Add {
			line.Old = oldNumber
			oldNumber++
		}
		if codeLine.kind != diff.Delete {
			line.New = newNumber
			newNumber++
		}
		hunk.Lines = append(hunk.Lines, line)
	}
	hunk.OldLines, hunk.NewLines = oldNumber-1, newNumber-1
	return &diff.File{OldPath: path, NewPath: path, Hunks: []diff.Hunk{hunk}}
}

// rowSpans shows file with the real Chroma highlighter and returns the spans
// drawn for each of its lines, in order.
func rowSpans(test *testing.T, file *diff.File) [][]Span {
	test.Helper()
	model := New(theme.New(true))
	model.SetFile(file)
	var spans [][]Span
	for i, row := range model.rows {
		if row.line != nil {
			spans = append(spans, model.codeSpans(i))
		}
	}
	return spans
}

// inKind reports whether tokenType is of the kind: a category (Comment) or
// a sub-category (LiteralString).
func inKind(tokenType, kind chroma.TokenType) bool {
	return tokenType.Category() == kind || tokenType.SubCategory() == kind
}

// allIn reports whether every non-blank span is in kind.
func allIn(spans []Span, kind chroma.TokenType) bool {
	for _, span := range spans {
		if strings.TrimSpace(span.Text) != "" && !inKind(span.Token, kind) {
			return false
		}
	}
	return len(spans) > 0
}

// anyIn reports whether some span is in kind.
func anyIn(spans []Span, kind chroma.TokenType) bool {
	for _, span := range spans {
		if inKind(span.Token, kind) {
			return true
		}
	}
	return false
}

func TestHunkIsHighlightedAsBlocks(test *testing.T) {
	long := strings.Repeat("x", maximumHighlightLength+1)
	// A side over the size cap, made of lines under the length limit.
	var huge []codeLine
	huge = append(huge, codeLine{diff.Add, "/* opened"})
	for range maximumBlockSize/400 + 1 {
		huge = append(huge, codeLine{diff.Add, "inside " + strings.Repeat("y", 490)})
	}
	huge = append(huge, codeLine{diff.Add, "after */"})

	tests := []struct {
		name  string
		path  string
		lines []codeLine
		// check returns a problem with the spans of the lines, or "".
		check func(spans [][]Span) string
	}{
		{
			name: "go block comment over three lines",
			path: "x.go",
			lines: []codeLine{
				{diff.Context, "x := 1"},
				{diff.Add, "/* start"},
				{diff.Add, " * it's the middle"},
				{diff.Add, " end */"},
				{diff.Context, "y := 2"},
			},
			check: func(spans [][]Span) string {
				switch {
				case anyIn(spans[0], chroma.Comment):
					return "the code line before is a comment"
				case !allIn(spans[1], chroma.Comment), !allIn(spans[2], chroma.Comment), !allIn(spans[3], chroma.Comment):
					return "a line of the comment is not a comment"
				case anyIn(spans[4], chroma.Comment):
					return "the code line after is a comment"
				}
				return ""
			},
		},
		{
			name: "python triple-quoted string",
			path: "x.py",
			lines: []codeLine{
				{diff.Add, `text = """first`},
				{diff.Add, `def not_code(): pass`},
				{diff.Add, `last"""`},
				{diff.Add, `after = 1`},
			},
			check: func(spans [][]Span) string {
				switch {
				case !allIn(spans[1], chroma.LiteralString), !allIn(spans[2], chroma.LiteralString):
					return "the middle or closing line is not a string"
				case anyIn(spans[3], chroma.LiteralString):
					return "the code line after is a string"
				}
				return ""
			},
		},
		{
			name: "comment opened on a deleted line and closed on a context line",
			path: "x.go",
			lines: []codeLine{
				{diff.Context, "a := 1"},
				{diff.Delete, "/* old"},
				{diff.Context, " end */"},
				{diff.Add, "b := 2"},
			},
			check: func(spans [][]Span) string {
				switch {
				case !allIn(spans[1], chroma.Comment):
					return "the deleted opening line is not a comment on the old side"
				case anyIn(spans[2], chroma.Comment):
					return "the context line is a comment, but nothing opens a comment on the new side"
				case anyIn(spans[3], chroma.Comment):
					return "the added line is a comment"
				}
				return ""
			},
		},
		{
			name: "comment opened on a deleted line and closed on a deleted line",
			path: "x.go",
			lines: []codeLine{
				{diff.Delete, "/* old"},
				{diff.Delete, " middle"},
				{diff.Delete, " end */"},
				{diff.Add, "b := 2"},
			},
			check: func(spans [][]Span) string {
				if !allIn(spans[1], chroma.Comment) || !allIn(spans[2], chroma.Comment) {
					return "the old comment is not whole"
				}
				if anyIn(spans[3], chroma.Comment) {
					return "the added line is a comment"
				}
				return ""
			},
		},
		{
			name: "a line over the length limit lexes the side line by line",
			path: "x.go",
			lines: []codeLine{
				{diff.Add, "/* opened"},
				{diff.Add, long},
				{diff.Add, "func f() {}"},
			},
			check: func(spans [][]Span) string {
				switch {
				case joined(spans[1]) != strings.Repeat("x", maximumHighlightLength+1) || len(spans[1]) != 1 || spans[1][0].Token != chroma.Text:
					return "the long line is not plain"
				case anyIn(spans[2], chroma.Comment) || !anyIn(spans[2], chroma.Keyword):
					return "the line after the long one is not lexed on its own"
				}
				return ""
			},
		},
		{
			name:  "a side over the size cap lexes line by line",
			path:  "x.go",
			lines: huge,
			check: func(spans [][]Span) string {
				if anyIn(spans[1], chroma.Comment) {
					return "the cap did not stop the block lexing"
				}
				return ""
			},
		},
		{
			name: "a file the lexer does not know is plain",
			path: "notes.unknownext",
			lines: []codeLine{
				{diff.Add, "/* start"},
				{diff.Add, "end */"},
			},
			check: func(spans [][]Span) string {
				for _, line := range spans {
					if len(line) != 1 || line[0].Token != chroma.Text {
						return "a line is not plain"
					}
				}
				return ""
			},
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			file := hunkFile(testCase.path, testCase.lines)
			spans := rowSpans(test, file)
			if len(spans) != len(testCase.lines) {
				test.Fatalf("%d span lists for %d lines", len(spans), len(testCase.lines))
			}
			for i, line := range testCase.lines {
				if got := joined(spans[i]); got != line.text {
					test.Fatalf("line %d: spans join to %q, want %q", i, got, line.text)
				}
			}
			if problem := testCase.check(spans); problem != "" {
				test.Error(problem)
				for i, line := range spans[:min(len(spans), 5)] {
					test.Logf("line %d: %.120v", i, line)
				}
			}
		})
	}
}

// TestHunkNoNewlineMarkerTakesNoPart proves a "no newline" marker is not
// lexed with the code and does not shift the spans of the lines around it.
func TestHunkNoNewlineMarkerTakesNoPart(test *testing.T) {
	file := hunkFile("x.go", []codeLine{
		{diff.Add, "/* open"},
		{diff.NoNewline, `\ No newline at end of file`},
		{diff.Add, "end */"},
	})
	var seen []string
	model := New(theme.New(true), WithHighlighter(func(path string, lines []string) [][]Span {
		seen = append(seen, lines...)
		return ChromaHighlighter()(path, lines)
	}))
	model.SetFile(file)
	for i, row := range model.rows {
		if row.line != nil {
			model.codeSpans(i)
		}
	}
	if got, want := strings.Join(seen, "|"), "/* open|end */"; got != want {
		test.Errorf("highlighted lines %q, want %q", got, want)
	}
}

// BenchmarkHighlightHunkSide compares lexing a side as one block with lexing
// its lines one at a time, without the size cap, to pick maximumBlockSize.
func BenchmarkHighlightHunkSide(benchmark *testing.B) {
	lexer := chroma.Coalesce(lexers.Match("x.go"))
	for _, count := range []int{1000, 2500, 5000} {
		lines := make([]string, count)
		for i := range lines {
			lines[i] = fmt.Sprintf("\tvalue%d := compute(%d, \"text\") // note %d", i, i, i)
		}
		benchmark.Run(fmt.Sprintf("block/%d", count), func(benchmark *testing.B) {
			for range benchmark.N {
				lexBlock(lexer, lines)
			}
		})
		benchmark.Run(fmt.Sprintf("perline/%d", count), func(benchmark *testing.B) {
			for range benchmark.N {
				for _, line := range lines {
					lexLine(lexer, line)
				}
			}
		})
	}
}

// BenchmarkRenderAddedFile5k draws the first screen of a 5k-line added file:
// its one side is over the size cap, so it is lexed as it is drawn.
func BenchmarkRenderAddedFile5k(benchmark *testing.B) {
	benchmarkRenderAddedFile(benchmark, 5000)
}

// BenchmarkRenderAddedFileUnderCap draws the first screen of an added file
// that fits the size cap, so its whole hunk is lexed as one block first.
func BenchmarkRenderAddedFileUnderCap(benchmark *testing.B) {
	benchmarkRenderAddedFile(benchmark, 2500)
}

func benchmarkRenderAddedFile(benchmark *testing.B, count int) {
	lines := make([]codeLine, count)
	for i := range lines {
		lines[i] = codeLine{diff.Add, fmt.Sprintf("\tvalue%d := compute(%d, \"text\") // note %d", i, i, i)}
	}
	file := hunkFile("x.go", lines)
	for range benchmark.N {
		model := New(theme.New(true))
		model.SetFile(file)
		model.SetSize(120, 50)
		model.Render(120, 50)
	}
}
