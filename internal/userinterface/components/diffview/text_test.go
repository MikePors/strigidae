package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDisplayText(test *testing.T) {
	tests := []struct {
		in    string
		want  string
		width int
	}{
		{"plain", "plain", 5},
		{"\tx", "    x", 5},
		{"ab\tc", "ab  c", 5},
		{"abcd\te", "abcd    e", 9},
		{"日本", "日本", 4},
		{"a\x1b[31mb", "a·[31mb", 7}, // escape sequences cannot reach the terminal
		{"", "", 0},
	}
	for _, testCase := range tests {
		got, width := displayText(testCase.in)
		if got != testCase.want || width != testCase.width {
			test.Errorf("displayText(%q) = %q, %d; want %q, %d", testCase.in, got, width, testCase.want, testCase.width)
		}
	}
}

func TestWindow(test *testing.T) {
	spans := []Span{{"ab", ClassKeyword}, {"日本語", ClassString}, {"cd", ClassPlain}}
	join := func(span []Span) string {
		var builder strings.Builder
		for _, piece := range span {
			builder.WriteString(piece.Text)
		}
		return builder.String()
	}
	tests := []struct {
		startColumn, width int
		want               string
	}{
		{0, 100, "ab日本語cd"},
		{0, 2, "ab"},
		{0, 3, "ab "}, // wide char does not fit: padded
		{0, 4, "ab日"},
		{1, 4, "b日 "},
		{3, 4, " 本 "}, // starts inside 日: its right half becomes a space
		{4, 3, "本 "},
		{10, 5, ""},
		{8, 5, "cd"},
	}
	emoji := []Span{{"❤️❤️a", ClassPlain}}
	for _, testCase := range []struct {
		startColumn, width int
		want               string
	}{
		{0, 4, "❤️❤️"},
		{0, 3, "❤️ "},
		{1, 3, " ❤️"},
		{2, 3, "❤️a"},
		{3, 2, " a"},
	} {
		got, used := window(emoji, testCase.startColumn, testCase.width)
		if joined := join(got); joined != testCase.want || used != ansi.StringWidth(joined) {
			test.Errorf("emoji window(%d,%d) = %q (used %d), want %q", testCase.startColumn, testCase.width, joined, used, testCase.want)
		}
	}
	for _, testCase := range tests {
		got, used := window(spans, testCase.startColumn, testCase.width)
		if joined := join(got); joined != testCase.want {
			test.Errorf("window(%d,%d) = %q, want %q", testCase.startColumn, testCase.width, joined, testCase.want)
		}
		if width := ansi.StringWidth(join(got)); width != used || used > testCase.width {
			test.Errorf("window(%d,%d): used %d, text width %d", testCase.startColumn, testCase.width, used, width)
		}
	}
}

func TestClassifyAndChroma(test *testing.T) {
	highlighter := ChromaHighlighter()
	spans := highlighter("x.go", "func main() { return 42 } // hi")
	if joined(spans) != "func main() { return 42 } // hi" {
		test.Fatalf("text altered: %v", spans)
	}
	has := func(class Class) bool {
		for _, span := range spans {
			if span.Class == class {
				return true
			}
		}
		return false
	}
	if !has(ClassKeyword) || !has(ClassNumber) || !has(ClassComment) {
		test.Errorf("expected keyword, number and comment spans, got %+v", spans)
	}
	if got := highlighter("notes.unknownext", "just text"); len(got) != 1 || got[0].Class != ClassPlain {
		test.Errorf("unknown extension should be plain: %+v", got)
	}
	if got := highlighter("x.go", ""); got != nil {
		test.Errorf("empty line: %+v", got)
	}
	long := strings.Repeat("x", maximumHighlightLength+1)
	if got := highlighter("x.go", long); len(got) != 1 || got[0].Class != ClassPlain {
		test.Error("very long lines should skip lexing")
	}
}
