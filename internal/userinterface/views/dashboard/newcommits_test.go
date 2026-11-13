package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
)

// TestNewCommitsGolden shows the glyph beside a decision glyph, alone, and not
// at all for a pull request reviewed at its head.
func TestNewCommitsGolden(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	review := pullRequest("acme/widgets", 12, "Cache the parsed diff", time.Hour)
	review.ReviewDecision = github.ReviewDecisionApproved
	review.HeadSHA, review.ViewerReviewedSHA = "new", "old"
	other := pullRequest("acme/gadgets", 7, "Fix the gadget", 3*time.Hour)
	other.HeadSHA, other.ViewerReviewedSHA = "new", "old"
	mine := pullRequest("rivka/dotfiles", 3, "Tidy the shell prompt", 26*time.Hour)
	mine.ReviewDecision = github.ReviewDecisionChangesRequested
	mine.HeadSHA, mine.ViewerReviewedSHA = "same", "same"
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{review, other})
	client.SetSearchResults(searchQuery(tabs[0].Sections[1]), []github.PullRequest{mine})
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	out := ansi.Strip(view.Render(100, 12))
	for i, line := range strings.Split(out, "\n") {
		if width := ansi.StringWidth(line); width > 100 {
			test.Errorf("line %d is %d columns wide, max 100", i, width)
		}
	}
	golden.Assert(test, "new_commits", out, false)
}
