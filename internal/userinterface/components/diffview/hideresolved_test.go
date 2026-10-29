package diffview

import (
	"strings"
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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
	if threads := model.Context().Threads; len(threads) != 0 {
		test.Errorf("Context threads on a line with only hidden threads = %v, want none", threads)
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
	if header < 0 {
		test.Fatal("the fading thread has no rows")
	}
	model.GotoTop()
	if model.fadeOf(header) != step {
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

func TestThreadFadeCombinesWithHunkFade(test *testing.T) {
	for name, testCase := range map[string]struct {
		hunkStep, threadStep int
		fading               bool
		want                 int
	}{
		"thread fade further":    {hunkStep: 1, threadStep: 3, fading: true, want: 3},
		"hunk fade further":      {hunkStep: 2, threadStep: 1, fading: true, want: 2},
		"thread not fading":      {hunkStep: 1, want: 1},
		"thread fade, hunk none": {threadStep: 2, fading: true, want: 2},
	} {
		test.Run(name, func(test *testing.T) {
			model := threadModel(test, 100, 80)
			model.SetHunkFade(func(*diff.File, int) int { return testCase.hunkStep })
			model.SetThreadFade(func(identifier string) (int, bool) {
				if identifier == "open" {
					return testCase.threadStep, testCase.fading
				}
				return 0, false
			})
			for i, row := range model.rows {
				if row.threadRow != nil && model.fileThreads[row.threadRow.threadIndex].Identifier == "open" {
					if got := model.fadeOf(i); got != testCase.want {
						test.Errorf("fadeOf = %d, want %d", got, testCase.want)
					}
					return
				}
			}
			test.Fatal("the open thread has no rows")
		})
	}
}

func TestHideResolvedOnNoticeOnlyFile(test *testing.T) {
	plain := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	plain.SetSize(60, 10)
	plain.SetFile(fixture(test, "logo.png"))
	want := screen(plain)

	model := New(theme.New(true), WithClock(func() time.Time { return threadNow }))
	model.SetSize(60, 10)
	model.SetFile(fixture(test, "logo.png"))
	resolved := binaryThread()
	resolved.IsResolved = true
	model.SetThreads([]github.ReviewThread{resolved})
	model.NextThread()
	fading := true
	model.SetThreadFade(func(string) (int, bool) { return 1, fading })

	model.SetHideResolved(true)
	if !strings.Contains(screen(model), "hank") {
		test.Errorf("a fading thread should keep the thread layout:\n%s", screen(model))
	}
	fading = false
	model.ThreadFadesChanged()
	if got := screen(model); got != want {
		test.Errorf("with every thread hidden the notice should look as with no threads:\n got:\n%s\nwant:\n%s", got, want)
	}
	if model.ThreadCount() != 0 {
		test.Errorf("ThreadCount = %d, want 0", model.ThreadCount())
	}
	model.NextThread() // nothing to reach; must not panic
	model.CursorDown(1)
	model.SetFile(fixture(test, "logo.png")) // a refresh while hidden
	model.SetThreads([]github.ReviewThread{resolved})
	if got := screen(model); got != want {
		test.Errorf("after a refresh the notice should still look as with no threads:\n%s", got)
	}

	model.SetHideResolved(false)
	out := screen(model)
	if !strings.Contains(out, "hank") || !strings.Contains(cursorRowText(model), "Binary file") {
		test.Errorf("turning the option off should bring the thread back with the cursor on the notice, at %q:\n%s", cursorRowText(model), out)
	}
}
