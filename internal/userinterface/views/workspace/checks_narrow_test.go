package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
)

// TestHeaderChecksSurviveTruncation pins the line order: with a long branch
// name at 80 columns, truncation eats the branches, not the check state.
func TestHeaderChecksSurviveTruncation(t *testing.T) {
	h := newHarness(t, 80, 24)
	d := testDetail()
	d.Author = "dependabot[bot]"
	d.BaseBranch, d.HeadBranch = "main", "dependabot/github_actions/actions-1c0c2ed584"
	d.ReviewDecision = github.ReviewDecisionReviewRequired
	d.Checks = github.ChecksPassing
	h.client.SetPullRequest(d)
	h.run(h.v.Init())
	line := strings.Split(ansi.Strip(h.render()), "\n")[1]
	for _, want := range []string{"dependabot[bot]", "✓ checks passing", "review required"} {
		if !strings.Contains(line, want) {
			t.Errorf("header line %q lacks %q", line, want)
		}
	}
}
