package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
)

// TestHeaderChecksSurviveTruncation pins the line order: with a long branch
// name at 80 columns, truncation eats the branches, not the check state.
func TestHeaderChecksSurviveTruncation(test *testing.T) {
	harness := newHarness(test, 80, 24)
	detail := testDetail()
	detail.Author = "dependabot[bot]"
	detail.BaseBranch, detail.HeadBranch = "main", "dependabot/github_actions/actions-1c0c2ed584"
	detail.ReviewDecision = github.ReviewDecisionReviewRequired
	detail.Checks = github.ChecksPassing
	harness.client.SetPullRequest(detail)
	harness.run(harness.v.Init())
	line := strings.Split(ansi.Strip(harness.render()), "\n")[1]
	for _, want := range []string{"dependabot[bot]", "✓ checks passing", "review required"} {
		if !strings.Contains(line, want) {
			test.Errorf("header line %q lacks %q", line, want)
		}
	}
}
