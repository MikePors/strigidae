package pullrequestlist

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
)

// TestNewCommitsGolden shows the glyph beside a decision glyph, alone, and not
// at all for a pull request reviewed at its head.
func TestNewCommitsGolden(test *testing.T) {
	pullRequests := makePullRequests(4)
	for i := range pullRequests {
		pullRequests[i].HeadSHA = "new"
	}
	pullRequests[0].ViewerReviewedSHA = "old"
	pullRequests[0].ReviewDecision = github.ReviewDecisionApproved
	pullRequests[1].ViewerReviewedSHA = "old"
	pullRequests[2].ViewerReviewedSHA = "new"
	pullRequests[2].ReviewDecision = github.ReviewDecisionChangesRequested
	view := markedView(test, pullRequests)
	for _, testCase := range []struct {
		name  string
		moves int
	}{{"new_commits", 0}, {"new_commits_selected", 1}} {
		for range testCase.moves {
			press(view, "j")
		}
		out := ansi.Strip(view.Render(80, 8))
		for i, line := range strings.Split(out, "\n") {
			if width := ansi.StringWidth(line); width > 80 {
				test.Errorf("%s: line %d is %d columns wide, max 80", testCase.name, i, width)
			}
		}
		golden.Assert(test, testCase.name, out, false)
	}
}
