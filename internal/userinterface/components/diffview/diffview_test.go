package diffview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
)

func fixture(test testing.TB, path string) *diff.File {
	test.Helper()
	bytes, err := os.ReadFile(filepath.Join("testdata", "sample.diff"))
	if err != nil {
		test.Fatal(err)
	}
	parsedDiff, err := diff.Parse(string(bytes))
	if err != nil {
		test.Fatal(err)
	}
	file := parsedDiff.File(path)
	if file == nil {
		test.Fatalf("fixture has no %s", path)
	}
	return file
}

func newModel(test testing.TB, width, height int) *Model {
	test.Helper()
	model := New(theme.New(true))
	model.SetFile(fixture(test, "server.go"))
	model.SetSize(width, height)
	return model
}

// Rows of server.go: 0 hunk 1 header, 1..11 its lines, 12 hunk 2 header,
// 13.. its lines. Row 15 is the first deleted line of hunk 2.
const (
	hunk2Row   = 12
	deletedRow = 15
	totalRows  = 25 // hunk 1: header + 11 lines; hunk 2: header + 12 lines (incl. marker)
)

func TestRowLayout(test *testing.T) {
	model := newModel(test, 80, 10)
	if len(model.rows) != totalRows {
		test.Fatalf("rows = %d", len(model.rows))
	}
	if got := model.hunkRow; len(got) != 2 || got[1] != hunk2Row {
		test.Fatalf("hunkRow = %v", got)
	}
}

// The movement rules live in the scroll package; these prove every exported
// method is wired to the right one.
func TestMovementDelegates(test *testing.T) {
	tests := []struct {
		name       string
		do         func(model *Model)
		wantCursor int
		wantTop    int
	}{
		{"CursorDown", func(model *Model) { model.CursorDown(8) }, 8, 3},
		{"CursorUp", func(model *Model) { model.CursorDown(8); model.CursorUp(2) }, 6, 3},
		{"GotoBottom", func(model *Model) { model.GotoBottom() }, totalRows - 1, totalRows - 6},
		{"GotoTop", func(model *Model) { model.GotoBottom(); model.GotoTop() }, 0, 0},
		{"HalfPageDown", func(model *Model) { model.HalfPageDown() }, 3, 3},
		{"HalfPageUp", func(model *Model) { model.PageDown(); model.HalfPageUp() }, 3, 3},
		{"PageDown", func(model *Model) { model.PageDown() }, 6, 6},
		{"PageUp", func(model *Model) { model.PageDown(); model.PageUp() }, 0, 0},
		{"ScrollDown pulls the cursor", func(model *Model) { model.ScrollDown(4) }, 4, 4},
		{"ScrollUp", func(model *Model) { model.GotoBottom(); model.ScrollUp(3) }, totalRows - 4, totalRows - 9},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := newModel(test, 80, 6)
			testCase.do(model)
			if model.window.Cursor() != testCase.wantCursor || model.Top() != testCase.wantTop {
				test.Errorf("cursor/top = %d/%d, want %d/%d", model.window.Cursor(), model.Top(), testCase.wantCursor, testCase.wantTop)
			}
		})
	}
}

func TestResizeKeepsCursorVisible(test *testing.T) {
	model := newModel(test, 80, 10)
	model.CursorDown(9)
	model.SetSize(80, 4)
	if model.window.Cursor() < model.window.Top() || model.window.Cursor() >= model.window.Top()+4 {
		test.Errorf("cursor %d outside [%d,%d)", model.window.Cursor(), model.window.Top(), model.window.Top()+4)
	}
}

// The code area, not the whole pane, is the horizontal view.
func TestHorizontalScrollUsesCodeWidth(test *testing.T) {
	model := newModel(test, 60, 8)
	maximumHorizontalOffset := model.maximumWidth - model.codeWidth()
	if maximumHorizontalOffset <= 0 {
		test.Fatalf("expected a line wider than the code area, maxX = %d", maximumHorizontalOffset)
	}
	model.ScrollRight(10000)
	if model.HorizontalOffset() != maximumHorizontalOffset {
		test.Errorf("right clamp: %d, want %d", model.HorizontalOffset(), maximumHorizontalOffset)
	}
	model.HalfScrollLeft()
	if want := maximumHorizontalOffset - model.codeWidth()/2; model.HorizontalOffset() != want {
		test.Errorf("half left: %d, want %d", model.HorizontalOffset(), want)
	}
	model.ScrollLeft(10000)
	model.HalfScrollRight()
	if model.HorizontalOffset() != model.codeWidth()/2 {
		test.Errorf("half right: %d, want %d", model.HorizontalOffset(), model.codeWidth()/2)
	}
	model.ScrollRight(3)
	model.ScrollLeft(1)
	if model.HorizontalOffset() != model.codeWidth()/2+2 {
		test.Errorf("right/left: %d", model.HorizontalOffset())
	}
	model.SetSize(200, 8) // everything fits: no scrolling possible
	if model.HorizontalOffset() != 0 {
		test.Errorf("wide pane: x=%d", model.HorizontalOffset())
	}
}

func TestHunkNavigation(test *testing.T) {
	model := newModel(test, 80, 6)
	model.NextHunk()
	if model.window.Cursor() != hunk2Row || model.window.Top() != hunk2Row {
		test.Errorf("next: cursor/top = %d/%d", model.window.Cursor(), model.window.Top())
	}
	model.NextHunk() // last hunk: no-op
	if model.window.Cursor() != hunk2Row {
		test.Errorf("next on last hunk moved to %d", model.window.Cursor())
	}
	model.CursorDown(3)
	model.PreviousHunk() // inside hunk 2: goes to its header
	if model.window.Cursor() != hunk2Row {
		test.Errorf("prev inside hunk: %d", model.window.Cursor())
	}
	model.PreviousHunk()
	if model.window.Cursor() != 0 {
		test.Errorf("prev: %d", model.window.Cursor())
	}
	model.PreviousHunk() // first hunk: no-op
	if model.window.Cursor() != 0 {
		test.Errorf("prev on first hunk moved to %d", model.window.Cursor())
	}
}

func TestCursorInfo(test *testing.T) {
	model := newModel(test, 80, 10)
	if _, ok := model.Cursor(); ok {
		test.Error("cursor on hunk header should report ok=false")
	}
	model.CursorDown(5) // "+\t\"log\""
	cursorLine, ok := model.Cursor()
	want := CursorLine{Path: "server.go", Hunk: 0, Kind: diff.Add, Old: 0, New: 5, Side: SideRight, Text: "\t\"log\""}
	if !ok || cursorLine != want || cursorLine.Number() != 5 {
		test.Errorf("add line: %+v ok=%v", cursorLine, ok)
	}
	model.GotoTop()
	model.CursorDown(deletedRow)
	cursorLine, ok = model.Cursor()
	if !ok || cursorLine.Kind != diff.Delete || cursorLine.Side != SideLeft || cursorLine.Old != 32 || cursorLine.New != 0 || cursorLine.Number() != 32 || cursorLine.Hunk != 1 {
		test.Errorf("delete line: %+v ok=%v", cursorLine, ok)
	}
	model.GotoBottom() // the no-newline marker
	if _, ok := model.Cursor(); ok {
		test.Error("marker row should not be commentable")
	}
	model.CursorUp(1)
	if cursorLine, ok := model.Cursor(); !ok || cursorLine.Kind != diff.Context || cursorLine.Side != SideRight || cursorLine.Old != 37 || cursorLine.New != 39 {
		test.Errorf("context line: %+v ok=%v", cursorLine, ok)
	}
	model.SetFile(nil)
	if _, ok := model.Cursor(); ok {
		test.Error("no file: ok should be false")
	}
}

func TestSetFileSameFileKeepsPosition(test *testing.T) {
	model := newModel(test, 80, 6)
	file := model.file
	model.CursorDown(7)
	model.SetFile(file)
	if model.window.Cursor() != 7 {
		test.Errorf("cursor reset to %d", model.window.Cursor())
	}
	model.SetFile(fixture(test, "pkg/new.go"))
	if model.window.Cursor() != 0 {
		test.Errorf("different path should reset cursor, got %d", model.window.Cursor())
	}
}

func TestRenderRespectsSize(test *testing.T) {
	for _, size := range [][2]int{{80, 14}, {30, 5}, {12, 3}, {5, 2}, {1, 1}, {0, 0}} {
		model := newModel(test, 80, 14)
		model.ScrollRight(7)
		out := model.Render(size[0], size[1])
		lines := strings.Split(out, "\n")
		if size[0] == 0 || size[1] == 0 {
			if out != "" {
				test.Errorf("%v: want empty, got %q", size, out)
			}
			continue
		}
		if len(lines) > size[1] {
			test.Errorf("%v: %d lines", size, len(lines))
		}
		for i, line := range lines {
			if width := ansi.StringWidth(line); width > size[0] {
				test.Errorf("%v: line %d is %d wide: %q", size, i, width, ansi.Strip(line))
			}
		}
	}
}

func TestStyledRows(test *testing.T) {
	model := newModel(test, 80, 14)
	rows := strings.Split(model.Render(80, 14), "\n")
	// Row 5 is added, row 9 deleted, row 1 context: their raw output differs in styling.
	if rows[5] == rows[1] || rows[9] == rows[5] {
		test.Error("add/delete/context rows should be styled differently")
	}
	if !strings.Contains(rows[0], "\x1b[") {
		test.Error("hunk header should be styled")
	}
}

func TestSpecialStates(test *testing.T) {
	model := New(theme.New(true))
	if got := ansi.Strip(model.Render(40, 3)); got != "No file selected." {
		test.Errorf("empty: %q", got)
	}
	model.SetDiffError(fmt.Errorf("get diff: %w", github.ErrDiffTooLarge))
	if got := ansi.Strip(model.Render(80, 5)); !strings.Contains(got, "too large") {
		test.Errorf("too large: %q", got)
	}
	model.SetDiffError(errors.New("boom"))
	if got := ansi.Strip(model.Render(80, 5)); !strings.Contains(got, "boom") {
		test.Errorf("error: %q", got)
	}
	model.SetFile(&diff.File{OldPath: "a", NewPath: "a", Status: diff.Modified})
	if got := ansi.Strip(model.Render(40, 3)); got != "No changes to show." {
		test.Errorf("no hunks: %q", got)
	}
	// Movement on a message must not panic.
	model.CursorDown(3)
	model.NextHunk()
	model.GotoBottom()
	model.ScrollRight(4)
	model.PageDown()
}

func TestGolden(test *testing.T) {
	tests := []struct {
		name          string
		path          string
		width, height int
		do            func(model *Model)
	}{
		{"normal", "server.go", 80, 25, nil},
		{"scrolled_right", "server.go", 60, 14, func(model *Model) { model.ScrollRight(48) }},
		{"cursor_on_deleted", "server.go", 70, 18, func(model *Model) { model.CursorDown(deletedRow) }},
		{"second_hunk", "server.go", 70, 8, func(model *Model) { model.NextHunk(); model.CursorDown(2) }},
		{"narrow", "server.go", 28, 12, nil},
		{"narrow_scrolled", "server.go", 28, 12, func(model *Model) { model.ScrollRight(30) }},
		{"binary", "logo.png", 50, 4, nil},
		{"rename_only", "pkg/new.go", 50, 4, nil},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := New(theme.New(true))
			model.SetFile(fixture(test, testCase.path))
			model.SetSize(testCase.width, testCase.height)
			if testCase.do != nil {
				testCase.do(model)
			}
			golden.Assert(test, testCase.name, model.Render(testCase.width, testCase.height), false)
		})
	}
	test.Run("too_large", func(test *testing.T) {
		model := New(theme.New(true))
		model.SetDiffError(github.ErrDiffTooLarge)
		golden.Assert(test, "too_large", model.Render(44, 5), false)
	})
	test.Run("no_file", func(test *testing.T) {
		golden.Assert(test, "no_file", New(theme.New(true)).Render(30, 3), false)
	})
}

// bigFile builds a single-hunk file of n context/add/delete lines.
func bigFile(count int) *diff.File {
	hunk := diff.Hunk{OldStart: 1, NewStart: 1}
	oldNumber, newNumber := 1, 1
	for i := range count {
		var line diff.Line
		switch i % 5 {
		case 3:
			line = diff.Line{Kind: diff.Delete, Old: oldNumber, Text: fmt.Sprintf("\tx := %d // old", i)}
			oldNumber++
		case 4:
			line = diff.Line{Kind: diff.Add, New: newNumber, Text: fmt.Sprintf("\tx := %d // new", i)}
			newNumber++
		default:
			line = diff.Line{Kind: diff.Context, Old: oldNumber, New: newNumber, Text: fmt.Sprintf("func f%d() int { return %d }", i, i)}
			oldNumber++
			newNumber++
		}
		hunk.Lines = append(hunk.Lines, line)
	}
	hunk.OldLines, hunk.NewLines = oldNumber-1, newNumber-1
	return &diff.File{OldPath: "big.go", NewPath: "big.go", Hunks: []diff.Hunk{hunk}}
}

func TestRenderOnlyHighlightsVisibleWindow(test *testing.T) {
	calls := 0
	inner := ChromaHighlighter()
	model := New(theme.New(true), WithHighlighter(func(path, text string) []Span {
		calls++
		return inner(path, text)
	}))
	model.SetFile(bigFile(5000))
	model.SetSize(100, 40)

	model.Render(100, 40)
	if calls != 39 {
		test.Fatalf("first render highlighted %d lines, want the 39 visible code rows", calls)
	}
	model.Render(100, 40)
	model.ScrollRight(10)
	model.Render(100, 40)
	if calls != 39 {
		test.Errorf("re-render and horizontal scroll re-highlighted: %d calls", calls)
	}
	model.CursorDown(1)
	model.ScrollDown(10)
	model.Render(100, 40)
	if calls != 49 {
		test.Errorf("scrolling by 10 should highlight exactly 10 new lines, got %d total", calls)
	}
	model.ScrollUp(10)
	model.Render(100, 40)
	if calls != 49 {
		test.Errorf("scrolling back should hit the cache, got %d total", calls)
	}
	model.GotoBottom()
	model.Render(100, 40)
	if calls > 100 {
		test.Errorf("jump to bottom highlighted %d lines in total", calls)
	}
}

func BenchmarkRenderViewport5k(benchmark *testing.B) {
	model := New(theme.New(true))
	model.SetFile(bigFile(5000))
	model.SetSize(120, 50)
	benchmark.ResetTimer()
	for i := range benchmark.N {
		model.ScrollDown(1 + i%3)
		if i%100 == 99 {
			model.GotoTop()
		}
		_ = model.Render(120, 50)
	}
}

func BenchmarkSetFile5k(benchmark *testing.B) {
	file := bigFile(5000)
	model := New(theme.New(true))
	for range benchmark.N {
		model.SetFile(nil)
		model.SetFile(file)
	}
}

// TestRenderUsesTokenTypeStyles proves the diff draws each token type with
// the theme's style for that type, not one per class: under tokyonight-night
// a declaration keyword ("func") and a plain keyword ("return") differ.
func TestRenderUsesTokenTypeStyles(test *testing.T) {
	tokyoNight, err := scheme.LoadBundled("tokyo-night")
	if err != nil || tokyoNight.Syntax != "tokyonight-night" {
		test.Fatalf("setup: %v, syntax %q", err, tokyoNight.Syntax)
	}
	activeTheme := tokyoNight.Theme()
	declaration, keyword := activeTheme.Syntax.For(chroma.KeywordDeclaration), activeTheme.Syntax.For(chroma.Keyword)
	if declaration.GetForeground() == keyword.GetForeground() {
		test.Fatal("setup: the style gives both keywords one colour")
	}
	model := New(activeTheme)
	model.SetFile(bigFile(3))
	model.SetSize(80, 4)
	model.GotoBottom() // keep the cursor's background off the first code row
	out := model.Render(80, 4)
	for _, want := range []string{declaration.Render("func"), keyword.Render("return")} {
		if !strings.Contains(out, want) {
			test.Errorf("render lacks %q:\n%q", want, out)
		}
	}
}
