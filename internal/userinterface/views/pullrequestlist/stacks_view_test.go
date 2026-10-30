package pullrequestlist

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// stackedPullRequests is a loaded list (newest update first) holding the stack
// 1 <- 2 <- 3, a sibling 5 of 2 on top of 1, and an unrelated #10. Drawn, the
// rows run 10, 1, 2, 3, 5.
func stackedPullRequests() []github.PullRequest {
	rows := []github.PullRequest{
		stacked(10, "docs/readme", "main"),
		stacked(3, "step-3", "step-2"),
		stacked(2, "step-2", "step-1"),
		stacked(1, "step-1", "main"),
		stacked(5, "sibling", "step-1"),
	}
	titles := map[int]string{10: "Tidy the readme", 3: "Third step", 2: "Second step", 1: "First step", 5: "Sibling of second"}
	for i := range rows {
		rows[i].Repository = testRepository
		rows[i].Title = titles[rows[i].Number]
		rows[i].Author = "alice"
		rows[i].UpdatedAt = testNow.Add(-time.Duration(i) * time.Hour)
	}
	rows[1].Draft = true
	return rows
}

func drawnNumbers(view *View) []int {
	var numbers []int
	for _, pullRequest := range view.current().pullRequests {
		numbers = append(numbers, pullRequest.Number)
	}
	return numbers
}

func TestStacksAreListedUnderTheirParents(test *testing.T) {
	view := loadedView(test, fakeWith(stackedPullRequests()), nil)
	if diff := cmp.Diff([]int{10, 1, 2, 3, 5}, drawnNumbers(view)); diff != "" {
		test.Errorf("drawn order mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]int{0, 0, 1, 2, 1}, view.current().depths); diff != "" {
		test.Errorf("depths mismatch (-want +got):\n%s", diff)
	}
}

func TestCursorAndEnterUseTheDrawnOrder(test *testing.T) {
	view := loadedView(test, fakeWith(stackedPullRequests()), nil)
	// Loaded order puts #3 in the second row; drawn order puts #1 there.
	press(view, "j")
	pullRequest, _ := view.current().selected()
	if pullRequest.Number != 1 {
		test.Fatalf("second row is #%d, want #1", pullRequest.Number)
	}
	pressCounted(view, "G", 4) // row 4 as drawn is #3
	push, ok := press(view, "enter")().(userinterface.PushMessage)
	if !ok || push.View.Title() != "#3" {
		test.Errorf("enter on row 4 pushed %v, want #3", push.View)
	}
	press(view, "G")
	if pullRequest, _ := view.current().selected(); pullRequest.Number != 5 {
		test.Errorf("last row is #%d, want #5", pullRequest.Number)
	}
}

func TestSearchWalksTheDrawnOrder(test *testing.T) {
	view := loadedView(test, fakeWith(stackedPullRequests()), nil)
	press(view, "/")
	typeSearch(view, "step")
	// "step" matches #1 (row 1), #2, #3 and #5 (via the head branch), not #10.
	if got := view.current().window.Cursor(); got != 1 {
		test.Fatalf("search cursor = %d, want 1", got)
	}
	finishSearch(view, "step", userinterface.PromptSubmitted)
	press(view, "n")
	press(view, "n")
	if pullRequest, _ := view.current().selected(); pullRequest.Number != 3 {
		test.Errorf("two matches on, cursor is on #%d, want #3 (loaded order would give #2)", pullRequest.Number)
	}
}

func TestReloadKeepsTheCursorOnTheSamePullRequest(test *testing.T) {
	client := fakeWith(stackedPullRequests())
	view := loadedView(test, client, nil)
	pressCounted(view, "G", 4) // #3
	deliver(test, view, press(view, "R"))
	if pullRequest, _ := view.current().selected(); pullRequest.Number != 3 || view.current().window.Cursor() != 3 {
		test.Errorf("after reload the cursor is on #%d at row %d, want #3 at row 3", pullRequest.Number, view.current().window.Cursor())
	}
}

func TestStackedGolden(test *testing.T) {
	view := loadedView(test, fakeWith(stackedPullRequests()), nil)
	press(view, "j")
	press(view, "j")
	out := view.Render(80, 9)
	for i, line := range strings.Split(out, "\n") {
		if width := ansi.StringWidth(line); width > 80 {
			test.Errorf("line %d is %d columns wide, max 80", i, width)
		}
	}
	golden.Assert(test, "stacked", out, false)
}
