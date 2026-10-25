package diffview

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
)

const firstPlaceholder = "▸ reviewed hunk hidden  @@ -1,9 +1,10 @@"

// hidingModel is server.go with its threads, hiding the hunks in hidden and
// fading them as far as fades says.
func hidingModel(test *testing.T, hidden map[int]bool, fades map[int]int) *Model {
	test.Helper()
	model := threadModel(test, 100, 80)
	model.SetHunkHidden(func(file *diff.File, hunk int) bool { return file == model.file && hidden[hunk] })
	model.SetHunkFade(func(_ *diff.File, hunk int) int { return fades[hunk] })
	return model
}

func TestHiddenHunkLeavesPlaceholder(test *testing.T) {
	model := hidingModel(test, map[int]bool{0: true}, nil)
	lines := strings.Split(screen(model), "\n")
	if got := strings.TrimRight(lines[0], " "); got != ">"+firstPlaceholder {
		test.Errorf("first row = %q, want the placeholder under the cursor", got)
	}
	shown := screen(model)
	for _, gone := range []string{"\"log\"", "Do we need", "Blank line please"} {
		if strings.Contains(shown, gone) {
			test.Errorf("the hidden hunk's %q is shown:\n%s", gone, shown)
		}
	}
	for _, kept := range []string{"@@ -30,8 +31,9 @@", "Why drop the return?", "Old thought.", "Where did this go?"} {
		if !strings.Contains(shown, kept) {
			test.Errorf("%q is missing:\n%s", kept, shown)
		}
	}
	if file, hunk, ok := model.CursorHunk(); !ok || file != model.file || hunk != 0 {
		test.Errorf("CursorHunk on the placeholder = %v, %d, %v; want hunk 0", file != nil, hunk, ok)
	}
	if _, ok := model.Cursor(); ok {
		test.Error("the placeholder is a line to comment on")
	}
	if model.ThreadCount() != 5 {
		test.Errorf("ThreadCount = %d, want every thread of the file", model.ThreadCount())
	}
}

func TestHunkNavigationSkipsHiddenHunks(test *testing.T) {
	model := hidingModel(test, map[int]bool{0: true}, nil)
	model.NextHunk()
	if got := cursorRowText(model); !strings.HasPrefix(got, ">@@ -30,8 +31,9 @@") {
		test.Fatalf("after ] the cursor row is %q, want the second hunk", got)
	}
	model.PreviousHunk()
	if got := cursorRowText(model); !strings.HasPrefix(got, ">@@ -30,8 +31,9 @@") {
		test.Errorf("[ moved to %q; the only hunk above is hidden", got)
	}

	model = hidingModel(test, map[int]bool{1: true}, nil)
	model.NextHunk()
	if got := model.CursorRow(); got != 0 {
		test.Errorf("] moved to row %d; the only hunk below is hidden", got)
	}
}

func TestGotoLineInHiddenHunkLandsOnPlaceholder(test *testing.T) {
	model := hidingModel(test, map[int]bool{0: true}, nil)
	model.GotoBottom()
	model.GotoLine(5)
	if got := strings.TrimRight(cursorRowText(model), " "); got != ">"+firstPlaceholder {
		test.Errorf("5G: cursor row %q, want the placeholder", got)
	}
	model.GotoLine(33)
	if cursor, ok := model.Cursor(); !ok || cursor.New != 33 {
		test.Errorf("33G: cursor %+v, want new line 33", cursor)
	}
}

// TestHidingMovesCursorToPlaceholder hides the hunk the cursor is in (the
// second, so its placeholder is not the first row), on a thread and on a
// line, and shows it again.
func TestHidingMovesCursorToPlaceholder(test *testing.T) {
	for name, target := range map[string]string{"line": "log.Println", "thread": "Why drop the return?"} {
		test.Run(name, func(test *testing.T) {
			hidden := map[int]bool{}
			model := hidingModel(test, hidden, nil)
			model.goToRowContaining(test, target)
			hidden[1] = true
			model.HiddenHunksChanged()
			if got := cursorRowText(model); !strings.HasPrefix(got, ">▸ reviewed hunk hidden  @@ -30,8 +31,9 @@") {
				test.Errorf("cursor row %q, want the placeholder", got)
			}
			hidden[1] = false
			model.HiddenHunksChanged()
			if got := cursorRowText(model); !strings.HasPrefix(got, ">@@ -30,8 +31,9 @@") {
				test.Errorf("shown again, cursor row %q, want the hunk's header", got)
			}
		})
	}
}

// goToRowContaining moves the cursor to the first row whose drawn text
// contains text, thread rows included.
func (model *Model) goToRowContaining(test *testing.T, text string) {
	test.Helper()
	for i := range model.rows {
		model.window.JumpTo(i)
		if strings.Contains(cursorRowText(model), text) {
			return
		}
	}
	test.Fatalf("no row contains %q", text)
}

func TestFadingHunkIsDrawnFainter(test *testing.T) {
	fades := map[int]int{}
	model := hidingModel(test, nil, fades)
	model.goToRowContaining(test, "\"log\"")
	normal := strings.Split(model.Render(100, 80), "\n")
	fades[0] = 2
	faded := strings.Split(model.Render(100, 80), "\n")
	if len(normal) != len(faded) {
		test.Fatalf("fading changed the row count: %d, then %d", len(normal), len(faded))
	}
	style := model.theme.Fade[1]
	for i, line := range normal {
		text := ansi.Strip(line)
		row := model.rows[model.Top()+i]
		inHunk := row.hunk == 0 && row.threadRow == nil
		if row.threadRow != nil {
			inHunk = !slices.Contains(model.orphans, row.threadRow.threadIndex) && model.base[row.baseIndex].hunk == 0
		}
		switch {
		case i+model.Top() == model.CursorRow():
			if faded[i] != line {
				test.Errorf("the cursor row %q faded", text)
			}
		case inHunk:
			if faded[i] != style.Render(text) {
				test.Errorf("row %d %q is not in fade step 2", i, text)
			}
		default:
			if faded[i] != line {
				test.Errorf("row %d %q outside the hunk changed", i, text)
			}
		}
	}
}

func TestHidingEndsSelectionInTheHunk(test *testing.T) {
	hidden := map[int]bool{}
	model := hidingModel(test, hidden, nil)
	model.goToRow(test, "\"log\"")
	if !model.VisualStart() {
		test.Fatal("setup: no selection")
	}
	hidden[1] = true
	model.HiddenHunksChanged()
	if !model.VisualActive() {
		test.Error("hiding another hunk ended the selection")
	}
	hidden[0] = true
	model.HiddenHunksChanged()
	if model.VisualActive() {
		test.Error("the selection outlived its hunk")
	}
}
