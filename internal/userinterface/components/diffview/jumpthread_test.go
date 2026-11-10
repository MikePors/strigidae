package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
)

func TestJumpThreadExactIdentityAndHiddenContent(test *testing.T) {
	for _, identifier := range []string{"open", "left", "resolved"} {
		test.Run(identifier, func(test *testing.T) {
			model := threadModel(test, 80, 18)
			model.SetHideResolved(true)
			model.SetHunkHidden(func(*diff.File, int) bool { return true })
			if !model.JumpThread(identifier) {
				test.Fatal("jump refused")
			}
			context := model.Context()
			if len(context.Threads) != 1 || context.Threads[0].Identifier != identifier {
				test.Fatalf("wrong thread: %#v", context.Threads)
			}
			if identifier == "left" && context.Threads[0].DiffSide != github.SideLeft {
				test.Fatal("lost deleted side")
			}
		})
	}
}

func TestJumpThreadRefusesUnmappedAnchor(test *testing.T) {
	model := threadModel(test, 80, 18)
	before := model.CursorRow()
	for _, identifier := range []string{"outdated", "gone", "other-file", "missing"} {
		if model.JumpThread(identifier) || model.CursorRow() != before {
			test.Errorf("%s jumped without a current anchor", identifier)
		}
	}
}

func TestJumpFileThread(test *testing.T) {
	model := threadModel(test, 80, 18)
	model.SetThreads([]github.ReviewThread{{Identifier: "file", Path: "server.go", SubjectType: github.SubjectFile}})
	if !model.JumpThread("file") || len(model.Context().Threads) != 1 {
		test.Fatal("file-level thread not reached")
	}
}

func TestJumpRevealEndsWhenHunksAreReviewed(test *testing.T) {
	model := threadModel(test, 80, 18)
	hidden := false
	model.SetHunkHidden(func(*diff.File, int) bool { return hidden })
	if !model.JumpThread("open") {
		test.Fatal("jump refused")
	}
	hidden = true
	model.HunkReviewedChanged(model.file, 0)
	model.HiddenHunksChanged()
	if strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("reviewed hunk stayed revealed after its fade")
	}
}

func TestJumpRevealEndsWhenThreadIsResolved(test *testing.T) {
	model := threadModel(test, 80, 18)
	model.SetHideResolved(true)
	if !model.JumpThread("open") {
		test.Fatal("jump refused")
	}
	threads := sampleThreads()
	threads[0].IsResolved = true
	model.SetThreadExpanded("open", false)
	model.SetThreads(threads)
	if strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("resolved thread stayed revealed")
	}
}

func TestJumpRevealSurvivesUnrelatedHunkNotifications(test *testing.T) {
	model := threadModel(test, 80, 18)
	model.SetHunkHidden(func(*diff.File, int) bool { return true })
	if !model.JumpThread("open") {
		test.Fatal("jump refused")
	}
	// A previous fade ending or an unrelated option invokes this general refresh.
	model.HiddenHunksChanged()
	if !strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("layout notification hid the jump target")
	}
	model.HunkReviewedChanged(model.file, 1)
	if !strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("reviewing a different hunk hid the jump target")
	}
	other := fixture(test, "pkg/new.go")
	model.HunkReviewedChanged(other, 0)
	if !strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("reviewing a different file hid the jump target")
	}
	model.HunkReviewedChanged(model.file, 0)
	if strings.Contains(ansi.Strip(model.Render(80, 18)), "Do we need") {
		test.Fatal("reviewing the target hunk did not end its reveal")
	}
}

func TestJumpRevealEndsOnHidingReconfiguration(test *testing.T) {
	model := threadModel(test, 80, 18)
	model.SetHideResolved(true)
	if !model.JumpThread("resolved") {
		test.Fatal("jump refused")
	}
	model.SetHideResolved(false)
	model.SetHideResolved(true)
	if !model.threadHidden(model.fileThreads[1]) {
		test.Fatal("hideresolved toggle did not clear the reveal")
	}
	if !model.JumpThread("resolved") {
		test.Fatal("second jump refused")
	}
	model.SetHunkHidden(func(*diff.File, int) bool { return true })
	if model.revealedThread != "" {
		test.Fatal("hidereviewed reconfiguration did not clear the reveal")
	}
}

// TestJumpThreadNearTheEndStopsAtTheLastPage: like the other thread jumps,
// a thread near the end of the file is not scrolled past the end.
func TestJumpThreadNearTheEndStopsAtTheLastPage(test *testing.T) {
	model := threadModel(test, 80, 30)
	if !model.JumpThread("left") {
		test.Fatal("JumpThread(left) found nothing")
	}
	if want := len(model.rows) - 30; model.CursorRow() <= want || model.Top() != want {
		test.Errorf("cursor/top = %d/%d, want the cursor below the last full page at top %d", model.CursorRow(), model.Top(), want)
	}
}
