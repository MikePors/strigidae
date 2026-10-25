package diffview

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestWrapColumns(test *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []int
	}{
		{"fits", "abc", 5, []int{0}},
		{"empty", "", 5, []int{0}},
		{"exact multiple", "abcdef", 3, []int{0, 3}},
		{"ascii", "abcdefgh", 3, []int{0, 3, 6}},
		{"wide character moves whole", "ab日本", 3, []int{0, 2, 4}},
		{"wider than the row gets its own row", "a日b", 1, []int{0, 1, 3}},
		{"combining mark stays with its letter", "abécd", 3, []int{0, 3}},
		{"no width", "abc", 0, []int{0}},
	}
	for _, testCase := range tests {
		if got := wrapColumns(testCase.text, testCase.width); !cmp.Equal(got, testCase.want) {
			test.Errorf("%s: wrapColumns(%q, %d) = %v, want %v", testCase.name, testCase.text, testCase.width, got, testCase.want)
		}
	}
}

// longLineFile has a short context line, a long added line and a short
// deleted line.
func longLineFile() *diff.File {
	hunk := diff.Hunk{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "short"},
		{Kind: diff.Add, New: 2, Text: "0123456789abcdefghijklmnopqrstuvwxyz"},
		{Kind: diff.Delete, Old: 2, Text: "gone"},
	}}
	return &diff.File{OldPath: "notes.txt", NewPath: "notes.txt", Hunks: []diff.Hunk{hunk}}
}

// wrapModel shows longLineFile wrapped, with a 10-cell code area (the gutter
// is 11 cells wide).
func wrapModel(test *testing.T, height int, options ...Option) *Model {
	test.Helper()
	model := New(theme.New(true), append([]Option{WithWrap(true)}, options...)...)
	model.SetFile(longLineFile())
	model.SetSize(21, height)
	if model.codeWidth() != 10 {
		test.Fatalf("setup: code width %d", model.codeWidth())
	}
	return model
}

func TestWrapRendersContinuationRows(test *testing.T) {
	model := wrapModel(test, 10)
	got := strings.Split(ansi.Strip(model.Render(21, 10)), "\n")
	want := []string{
		">@@ -1,2 +1,2 @@     ",
		"   1   1   short     ",
		"       2  +0123456789",
		"       ↪   abcdefghij",
		"       ↪   klmnopqrst",
		"       ↪   uvwxyz    ",
		"   2      -gone      ",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("wrapped screen (-want +got):\n%s", diff)
	}
}

func TestWrapKeepsTintAndHighlighting(test *testing.T) {
	highlighter := func(_, text string) []Span {
		if len(text) < 30 {
			return []Span{{Text: text, Token: chroma.Text}}
		}
		return []Span{{Text: text[:5], Token: chroma.Keyword}, {Text: text[5:], Token: chroma.LiteralString}}
	}
	model := wrapModel(test, 10, WithHighlighter(highlighter))
	rows := strings.Split(model.Render(21, 10), "\n")
	added := model.styles.variants[0][kindAdd]
	if continued := rows[3]; !strings.HasPrefix(continued, added.gutter.Render(" "+spaces(3)+" "+spaces(2)+markerContinued+"   ")) {
		test.Errorf("continuation gutter is not the added line's gutter: %q", continued)
	}
	// The string token started on the first row; its later rows keep its style.
	for _, row := range []struct {
		index int
		text  string
	}{{3, "abcdefghij"}, {4, "klmnopqrst"}} {
		if want := added.codeStyle(chroma.LiteralString).Render(row.text); !strings.Contains(rows[row.index], want) {
			test.Errorf("row %d lacks %q in the string style:\n%q", row.index, row.text, rows[row.index])
		}
	}
	if pad := added.fill.Render(spaces(4)); !strings.HasSuffix(rows[5], pad) {
		test.Errorf("the last continuation row is not padded with the added tint: %q", rows[5])
	}
}

func TestWrapCursorMovesByLine(test *testing.T) {
	model := wrapModel(test, 4)
	model.CursorDown(2) // header -> short -> the long line
	if line, ok := model.Cursor(); !ok || line.New != 2 {
		test.Fatalf("cursor on %+v, want the long line", line)
	}
	// The long line takes 4 rows, so the 4-row view starts at it.
	if model.Top() != 2 {
		test.Errorf("top = %d, want the long line's row so all of it shows", model.Top())
	}
	screen := strings.Split(ansi.Strip(model.Render(21, 4)), "\n")
	if !strings.HasPrefix(screen[0], ">      2") || !strings.Contains(screen[3], "uvwxyz") {
		test.Errorf("the cursor line is not fully visible:\n%s", strings.Join(screen, "\n"))
	}
	model.CursorDown(1)
	if line, _ := model.Cursor(); line.Kind != diff.Delete {
		test.Errorf("j from the long line went to %+v, want the next diff line", line)
	}
}

func TestWrapDisablesHorizontalScroll(test *testing.T) {
	model := wrapModel(test, 10)
	model.ScrollRight(5)
	model.HalfScrollRight()
	if got := model.HorizontalOffset(); got != 0 {
		test.Errorf("horizontal offset %d while wrapping, want 0", got)
	}
	model.SetWrap(false)
	model.ScrollRight(5)
	if got := model.HorizontalOffset(); got != 5 {
		test.Errorf("horizontal offset %d after turning wrapping off, want 5", got)
	}
	if strings.Contains(ansi.Strip(model.Render(21, 10)), markerContinued) {
		test.Error("continuation rows remain after turning wrapping off")
	}
}

// TestWrapMeasuresOnlyNearTheView: rows are measured when the scroll window
// or the renderer asks, which is near the view, never the whole file.
func TestWrapMeasuresOnlyNearTheView(test *testing.T) {
	model := New(theme.New(true), WithWrap(true))
	model.SetFile(bigFile(5000))
	model.SetSize(40, 20)
	model.Render(40, 20)
	model.GotoBottom()
	model.Render(40, 20)
	if measured := len(model.wrapStarts); measured == 0 || measured > 100 {
		test.Errorf("measured %d of 5000 rows, want only rows near the view", measured)
	}
}

// TestWrapVisualSelection: a selection covers diff lines, and every screen
// row of a selected wrapped line carries the selection's tint.
func TestWrapVisualSelection(test *testing.T) {
	model := wrapModel(test, 10)
	model.CursorDown(1) // the short context line
	if !model.VisualStart() {
		test.Fatal("visual mode did not start")
	}
	if err := model.VisualMove(1); err != nil {
		test.Fatal(err)
	}
	got, ok := model.Anchor()
	if !ok || got.Start != 1 || got.End != 2 {
		test.Errorf("selection = %+v, want new lines 1 to 2", got)
	}
	rows := strings.Split(model.Render(21, 10), "\n")
	tinted := model.styles.variants[1][kindAdd].gutter.Render(" " + spaces(3) + " " + spaces(2) + markerContinued + "   ")
	for _, index := range []int{3, 4, 5} {
		if !strings.HasPrefix(rows[index], tinted) {
			test.Errorf("continuation row %d lacks the cursor tint: %q", index, rows[index])
		}
	}
}

func TestWrapFollowsWidth(test *testing.T) {
	model := wrapModel(test, 10)
	model.Render(21, 10)
	screen := ansi.Strip(model.Render(26, 10)) // a 15-cell code area
	for _, want := range []string{"+0123456789abcde", "↪   fghijklmnopqrst", "↪   uvwxyz"} {
		if !strings.Contains(screen, want) {
			test.Errorf("after widening, screen lacks %q:\n%s", want, screen)
		}
	}
}
