package diffview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func fixture(t testing.TB, path string) *diff.File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sample.diff"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := diff.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	f := d.File(path)
	if f == nil {
		t.Fatalf("fixture has no %s", path)
	}
	return f
}

func newModel(t testing.TB, w, h int) *Model {
	t.Helper()
	m := New(theme.New(true))
	m.SetFile(fixture(t, "server.go"))
	m.SetSize(w, h)
	return m
}

// Rows of server.go: 0 hunk 1 header, 1..11 its lines, 12 hunk 2 header,
// 13.. its lines. Row 15 is the first deleted line of hunk 2.
const (
	hunk2Row   = 12
	deletedRow = 15
	totalRows  = 25 // hunk 1: header + 11 lines; hunk 2: header + 12 lines (incl. marker)
)

func TestRowLayout(t *testing.T) {
	m := newModel(t, 80, 10)
	if len(m.rows) != totalRows {
		t.Fatalf("rows = %d", len(m.rows))
	}
	if got := m.hunkRow; len(got) != 2 || got[1] != hunk2Row {
		t.Fatalf("hunkRow = %v", got)
	}
}

// The movement rules live in the scroll package; these prove every exported
// method is wired to the right one.
func TestMovementDelegates(t *testing.T) {
	tests := []struct {
		name       string
		do         func(m *Model)
		wantCursor int
		wantTop    int
	}{
		{"CursorDown", func(m *Model) { m.CursorDown(8) }, 8, 3},
		{"CursorUp", func(m *Model) { m.CursorDown(8); m.CursorUp(2) }, 6, 3},
		{"GotoBottom", func(m *Model) { m.GotoBottom() }, totalRows - 1, totalRows - 6},
		{"GotoTop", func(m *Model) { m.GotoBottom(); m.GotoTop() }, 0, 0},
		{"HalfPageDown", func(m *Model) { m.HalfPageDown() }, 3, 3},
		{"HalfPageUp", func(m *Model) { m.PageDown(); m.HalfPageUp() }, 3, 3},
		{"PageDown", func(m *Model) { m.PageDown() }, 6, 6},
		{"PageUp", func(m *Model) { m.PageDown(); m.PageUp() }, 0, 0},
		{"ScrollDown pulls the cursor", func(m *Model) { m.ScrollDown(4) }, 4, 4},
		{"ScrollUp", func(m *Model) { m.GotoBottom(); m.ScrollUp(3) }, totalRows - 4, totalRows - 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newModel(t, 80, 6)
			tt.do(m)
			if m.win.Cursor() != tt.wantCursor || m.Top() != tt.wantTop {
				t.Errorf("cursor/top = %d/%d, want %d/%d", m.win.Cursor(), m.Top(), tt.wantCursor, tt.wantTop)
			}
		})
	}
}

func TestResizeKeepsCursorVisible(t *testing.T) {
	m := newModel(t, 80, 10)
	m.CursorDown(9)
	m.SetSize(80, 4)
	if m.win.Cursor() < m.win.Top() || m.win.Cursor() >= m.win.Top()+4 {
		t.Errorf("cursor %d outside [%d,%d)", m.win.Cursor(), m.win.Top(), m.win.Top()+4)
	}
}

// The code area, not the whole pane, is the horizontal view.
func TestHorizontalScrollUsesCodeWidth(t *testing.T) {
	m := newModel(t, 60, 8)
	maxX := m.maxWidth - m.codeWidth()
	if maxX <= 0 {
		t.Fatalf("expected a line wider than the code area, maxX = %d", maxX)
	}
	m.ScrollRight(10000)
	if m.HorizontalOffset() != maxX {
		t.Errorf("right clamp: %d, want %d", m.HorizontalOffset(), maxX)
	}
	m.HalfScrollLeft()
	if want := maxX - m.codeWidth()/2; m.HorizontalOffset() != want {
		t.Errorf("half left: %d, want %d", m.HorizontalOffset(), want)
	}
	m.ScrollLeft(10000)
	m.HalfScrollRight()
	if m.HorizontalOffset() != m.codeWidth()/2 {
		t.Errorf("half right: %d, want %d", m.HorizontalOffset(), m.codeWidth()/2)
	}
	m.ScrollRight(3)
	m.ScrollLeft(1)
	if m.HorizontalOffset() != m.codeWidth()/2+2 {
		t.Errorf("right/left: %d", m.HorizontalOffset())
	}
	m.SetSize(200, 8) // everything fits: no scrolling possible
	if m.HorizontalOffset() != 0 {
		t.Errorf("wide pane: x=%d", m.HorizontalOffset())
	}
}

func TestHunkNavigation(t *testing.T) {
	m := newModel(t, 80, 6)
	m.NextHunk()
	if m.win.Cursor() != hunk2Row || m.win.Top() != hunk2Row {
		t.Errorf("next: cursor/top = %d/%d", m.win.Cursor(), m.win.Top())
	}
	m.NextHunk() // last hunk: no-op
	if m.win.Cursor() != hunk2Row {
		t.Errorf("next on last hunk moved to %d", m.win.Cursor())
	}
	m.CursorDown(3)
	m.PreviousHunk() // inside hunk 2: goes to its header
	if m.win.Cursor() != hunk2Row {
		t.Errorf("prev inside hunk: %d", m.win.Cursor())
	}
	m.PreviousHunk()
	if m.win.Cursor() != 0 {
		t.Errorf("prev: %d", m.win.Cursor())
	}
	m.PreviousHunk() // first hunk: no-op
	if m.win.Cursor() != 0 {
		t.Errorf("prev on first hunk moved to %d", m.win.Cursor())
	}
}

func TestCursorInfo(t *testing.T) {
	m := newModel(t, 80, 10)
	if _, ok := m.Cursor(); ok {
		t.Error("cursor on hunk header should report ok=false")
	}
	m.CursorDown(5) // "+\t\"log\""
	c, ok := m.Cursor()
	want := CursorLine{Path: "server.go", Hunk: 0, Kind: diff.Add, Old: 0, New: 5, Side: SideRight, Text: "\t\"log\""}
	if !ok || c != want || c.Number() != 5 {
		t.Errorf("add line: %+v ok=%v", c, ok)
	}
	m.GotoTop()
	m.CursorDown(deletedRow)
	c, ok = m.Cursor()
	if !ok || c.Kind != diff.Delete || c.Side != SideLeft || c.Old != 32 || c.New != 0 || c.Number() != 32 || c.Hunk != 1 {
		t.Errorf("delete line: %+v ok=%v", c, ok)
	}
	m.GotoBottom() // the no-newline marker
	if _, ok := m.Cursor(); ok {
		t.Error("marker row should not be commentable")
	}
	m.CursorUp(1)
	if c, ok := m.Cursor(); !ok || c.Kind != diff.Context || c.Side != SideRight || c.Old != 37 || c.New != 39 {
		t.Errorf("context line: %+v ok=%v", c, ok)
	}
	m.SetFile(nil)
	if _, ok := m.Cursor(); ok {
		t.Error("no file: ok should be false")
	}
}

func TestSetFileSameFileKeepsPosition(t *testing.T) {
	m := newModel(t, 80, 6)
	f := m.file
	m.CursorDown(7)
	m.SetFile(f)
	if m.win.Cursor() != 7 {
		t.Errorf("cursor reset to %d", m.win.Cursor())
	}
	m.SetFile(fixture(t, "pkg/new.go"))
	if m.win.Cursor() != 0 {
		t.Errorf("different path should reset cursor, got %d", m.win.Cursor())
	}
}

func TestRenderRespectsSize(t *testing.T) {
	for _, size := range [][2]int{{80, 14}, {30, 5}, {12, 3}, {5, 2}, {1, 1}, {0, 0}} {
		m := newModel(t, 80, 14)
		m.ScrollRight(7)
		out := m.Render(size[0], size[1])
		lines := strings.Split(out, "\n")
		if size[0] == 0 || size[1] == 0 {
			if out != "" {
				t.Errorf("%v: want empty, got %q", size, out)
			}
			continue
		}
		if len(lines) > size[1] {
			t.Errorf("%v: %d lines", size, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Errorf("%v: line %d is %d wide: %q", size, i, w, ansi.Strip(l))
			}
		}
	}
}

func TestStyledRows(t *testing.T) {
	m := newModel(t, 80, 14)
	rows := strings.Split(m.Render(80, 14), "\n")
	// Row 5 is added, row 9 deleted, row 1 context: their raw output differs in styling.
	if rows[5] == rows[1] || rows[9] == rows[5] {
		t.Error("add/delete/context rows should be styled differently")
	}
	if !strings.Contains(rows[0], "\x1b[") {
		t.Error("hunk header should be styled")
	}
}

func TestSpecialStates(t *testing.T) {
	m := New(theme.New(true))
	if got := ansi.Strip(m.Render(40, 3)); got != "No file selected." {
		t.Errorf("empty: %q", got)
	}
	m.SetDiffError(fmt.Errorf("get diff: %w", github.ErrDiffTooLarge))
	if got := ansi.Strip(m.Render(80, 5)); !strings.Contains(got, "too large") {
		t.Errorf("too large: %q", got)
	}
	m.SetDiffError(errors.New("boom"))
	if got := ansi.Strip(m.Render(80, 5)); !strings.Contains(got, "boom") {
		t.Errorf("error: %q", got)
	}
	m.SetFile(&diff.File{OldPath: "a", NewPath: "a", Status: diff.Modified})
	if got := ansi.Strip(m.Render(40, 3)); got != "No changes to show." {
		t.Errorf("no hunks: %q", got)
	}
	// Movement on a message must not panic.
	m.CursorDown(3)
	m.NextHunk()
	m.GotoBottom()
	m.ScrollRight(4)
	m.PageDown()
}

func TestGolden(t *testing.T) {
	tests := []struct {
		name string
		path string
		w, h int
		do   func(m *Model)
	}{
		{"normal", "server.go", 80, 25, nil},
		{"scrolled_right", "server.go", 60, 14, func(m *Model) { m.ScrollRight(48) }},
		{"cursor_on_deleted", "server.go", 70, 18, func(m *Model) { m.CursorDown(deletedRow) }},
		{"second_hunk", "server.go", 70, 8, func(m *Model) { m.NextHunk(); m.CursorDown(2) }},
		{"narrow", "server.go", 28, 12, nil},
		{"narrow_scrolled", "server.go", 28, 12, func(m *Model) { m.ScrollRight(30) }},
		{"binary", "logo.png", 50, 4, nil},
		{"rename_only", "pkg/new.go", 50, 4, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(theme.New(true))
			m.SetFile(fixture(t, tt.path))
			m.SetSize(tt.w, tt.h)
			if tt.do != nil {
				tt.do(m)
			}
			golden.Assert(t, tt.name, m.Render(tt.w, tt.h), false)
		})
	}
	t.Run("too_large", func(t *testing.T) {
		m := New(theme.New(true))
		m.SetDiffError(github.ErrDiffTooLarge)
		golden.Assert(t, "too_large", m.Render(44, 5), false)
	})
	t.Run("no_file", func(t *testing.T) {
		golden.Assert(t, "no_file", New(theme.New(true)).Render(30, 3), false)
	})
}

// bigFile builds a single-hunk file of n context/add/delete lines.
func bigFile(n int) *diff.File {
	h := diff.Hunk{OldStart: 1, NewStart: 1}
	oldNo, newNo := 1, 1
	for i := range n {
		var l diff.Line
		switch i % 5 {
		case 3:
			l = diff.Line{Kind: diff.Delete, Old: oldNo, Text: fmt.Sprintf("\tx := %d // old", i)}
			oldNo++
		case 4:
			l = diff.Line{Kind: diff.Add, New: newNo, Text: fmt.Sprintf("\tx := %d // new", i)}
			newNo++
		default:
			l = diff.Line{Kind: diff.Context, Old: oldNo, New: newNo, Text: fmt.Sprintf("func f%d() int { return %d }", i, i)}
			oldNo++
			newNo++
		}
		h.Lines = append(h.Lines, l)
	}
	h.OldLines, h.NewLines = oldNo-1, newNo-1
	return &diff.File{OldPath: "big.go", NewPath: "big.go", Hunks: []diff.Hunk{h}}
}

func TestRenderOnlyHighlightsVisibleWindow(t *testing.T) {
	calls := 0
	inner := ChromaHighlighter()
	m := New(theme.New(true), WithHighlighter(func(path, text string) []Span {
		calls++
		return inner(path, text)
	}))
	m.SetFile(bigFile(5000))
	m.SetSize(100, 40)

	m.Render(100, 40)
	if calls != 39 {
		t.Fatalf("first render highlighted %d lines, want the 39 visible code rows", calls)
	}
	m.Render(100, 40)
	m.ScrollRight(10)
	m.Render(100, 40)
	if calls != 39 {
		t.Errorf("re-render and horizontal scroll re-highlighted: %d calls", calls)
	}
	m.CursorDown(1)
	m.ScrollDown(10)
	m.Render(100, 40)
	if calls != 49 {
		t.Errorf("scrolling by 10 should highlight exactly 10 new lines, got %d total", calls)
	}
	m.ScrollUp(10)
	m.Render(100, 40)
	if calls != 49 {
		t.Errorf("scrolling back should hit the cache, got %d total", calls)
	}
	m.GotoBottom()
	m.Render(100, 40)
	if calls > 100 {
		t.Errorf("jump to bottom highlighted %d lines in total", calls)
	}
}

func BenchmarkRenderViewport5k(b *testing.B) {
	m := New(theme.New(true))
	m.SetFile(bigFile(5000))
	m.SetSize(120, 50)
	b.ResetTimer()
	for i := range b.N {
		m.ScrollDown(1 + i%3)
		if i%100 == 99 {
			m.GotoTop()
		}
		_ = m.Render(120, 50)
	}
}

func BenchmarkSetFile5k(b *testing.B) {
	f := bigFile(5000)
	m := New(theme.New(true))
	for range b.N {
		m.SetFile(nil)
		m.SetFile(f)
	}
}
