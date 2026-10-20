package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDisplayText(t *testing.T) {
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
	for _, tt := range tests {
		got, w := displayText(tt.in)
		if got != tt.want || w != tt.width {
			t.Errorf("displayText(%q) = %q, %d; want %q, %d", tt.in, got, w, tt.want, tt.width)
		}
	}
}

func TestWindow(t *testing.T) {
	spans := []Span{{"ab", ClassKeyword}, {"日本語", ClassString}, {"cd", ClassPlain}}
	join := func(s []Span) string {
		var b strings.Builder
		for _, sp := range s {
			b.WriteString(sp.Text)
		}
		return b.String()
	}
	tests := []struct {
		off, width int
		want       string
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
	for _, tt := range []struct {
		off, width int
		want       string
	}{
		{0, 4, "❤️❤️"},
		{0, 3, "❤️ "},
		{1, 3, " ❤️"},
		{2, 3, "❤️a"},
		{3, 2, " a"},
	} {
		got, used := window(emoji, tt.off, tt.width)
		if j := join(got); j != tt.want || used != ansi.StringWidth(j) {
			t.Errorf("emoji window(%d,%d) = %q (used %d), want %q", tt.off, tt.width, j, used, tt.want)
		}
	}
	for _, tt := range tests {
		got, used := window(spans, tt.off, tt.width)
		if j := join(got); j != tt.want {
			t.Errorf("window(%d,%d) = %q, want %q", tt.off, tt.width, j, tt.want)
		}
		if w := ansi.StringWidth(join(got)); w != used || used > tt.width {
			t.Errorf("window(%d,%d): used %d, text width %d", tt.off, tt.width, used, w)
		}
	}
}

func TestClassifyAndChroma(t *testing.T) {
	h := ChromaHighlighter()
	spans := h("x.go", "func main() { return 42 } // hi")
	if joined(spans) != "func main() { return 42 } // hi" {
		t.Fatalf("text altered: %v", spans)
	}
	has := func(c Class) bool {
		for _, s := range spans {
			if s.Class == c {
				return true
			}
		}
		return false
	}
	if !has(ClassKeyword) || !has(ClassNumber) || !has(ClassComment) {
		t.Errorf("expected keyword, number and comment spans, got %+v", spans)
	}
	if got := h("notes.unknownext", "just text"); len(got) != 1 || got[0].Class != ClassPlain {
		t.Errorf("unknown extension should be plain: %+v", got)
	}
	if got := h("x.go", ""); got != nil {
		t.Errorf("empty line: %+v", got)
	}
	long := strings.Repeat("x", maxHighlightLen+1)
	if got := h("x.go", long); len(got) != 1 || got[0].Class != ClassPlain {
		t.Error("very long lines should skip lexing")
	}
}
