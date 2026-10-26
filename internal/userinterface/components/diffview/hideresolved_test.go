package diffview

import (
	"strings"
	"testing"
)

func TestHideResolvedLeavesThreadOutButKeepsMarker(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.NextThread() // carol's resolved thread, the first one
	if !strings.Contains(cursorRowText(model), "carol") {
		test.Fatalf("cursor should start on the resolved thread, at %q", cursorRowText(model))
	}

	model.SetHideResolved(true)
	out := screen(model)
	if strings.Contains(out, "carol") || strings.Contains(out, "[resolved]") {
		test.Errorf("resolved thread still shown:\n%s", out)
	}
	if model.ThreadCount() != 4 {
		test.Errorf("ThreadCount = %d, want 4", model.ThreadCount())
	}
	if row := model.rows[model.window.Cursor()]; row.threadRow != nil || row.line == nil || row.line.New != 2 {
		test.Errorf("cursor should move to the hidden thread's line, at %q", cursorRowText(model))
	}
	var marked []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, markerOpen) || strings.Contains(line, markerQuiet) {
			marked = append(marked, line)
		}
	}
	if len(marked) != 3 || !strings.Contains(marked[0], markerQuiet) {
		test.Errorf("the hidden thread's line should keep its hollow marker:\n%s", strings.Join(marked, "\n"))
	}
	if threads := model.ThreadsAtCursor(); len(threads) != 0 {
		test.Errorf("ThreadsAtCursor on a line with only hidden threads = %v, want none", threads)
	}
	before := len(model.rows)
	model.ToggleThread()
	if len(model.rows) != before {
		test.Error("toggling on a line with only hidden threads changed the rows")
	}

	model.GotoTop()
	var seen []string
	for range 6 {
		model.NextThread()
		seen = append(seen, cursorRowText(model))
	}
	if joined := strings.Join(seen, "\n"); strings.Contains(joined, "carol") || !strings.Contains(joined, "alice") {
		test.Errorf("thread navigation should skip the hidden thread:\n%s", joined)
	}

	model.SetHideResolved(false)
	if out := screen(model); !strings.Contains(out, "▸ carol 1 comment · 2d ago [resolved]") || model.ThreadCount() != 5 {
		test.Errorf("turning the option off should show the resolved thread again:\n%s", out)
	}
}
