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
	model.ToggleThread()
	if _, chosen := model.open["resolved"]; chosen {
		test.Error("toggling on a line with only hidden threads unfolded the hidden thread")
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

func TestFadingResolvedThreadStaysUntilFadesChange(test *testing.T) {
	model := threadModel(test, 100, 80)
	fading, step := true, 2
	model.SetThreadFade(func(identifier string) (int, bool) {
		if identifier == "resolved" && fading {
			return step, true
		}
		return 0, false
	})
	model.SetHideResolved(true)
	if !strings.Contains(screen(model), "carol") {
		test.Fatalf("a fading resolved thread should stay shown:\n%s", screen(model))
	}
	header := -1
	for i, row := range model.rows {
		if row.threadRow != nil && model.fileThreads[row.threadRow.threadIndex].Identifier == "resolved" {
			header = i
			break
		}
	}
	model.GotoTop()
	if header < 0 || model.fadeOf(header) != step {
		test.Errorf("the fading thread's row fades by %d, want %d", model.fadeOf(header), step)
	}
	model.window.JumpTo(header)
	if model.fadeOf(header) != 0 {
		test.Error("the cursor row should never fade")
	}

	fading = false
	if !strings.Contains(screen(model), "carol") {
		test.Error("the thread should stay until ThreadFadesChanged")
	}
	model.ThreadFadesChanged()
	if strings.Contains(screen(model), "carol") {
		test.Errorf("the thread should leave once its fade ended:\n%s", screen(model))
	}
	if row := model.rows[model.window.Cursor()]; row.threadRow != nil || row.line == nil || row.line.New != 2 {
		test.Errorf("cursor should move to the thread's line, at %q", cursorRowText(model))
	}
}
