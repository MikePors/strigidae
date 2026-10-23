package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestHeaderChecks(test *testing.T) {
	activeTheme := theme.New(true)
	tests := []struct {
		state github.CheckState
		text  string
		style func(string) string
	}{
		{github.ChecksPassing, "✓ checks passing", func(text string) string { return activeTheme.PullRequestList.ChecksPassing.Render(text) }},
		{github.ChecksFailing, "✗ checks failing", func(text string) string { return activeTheme.PullRequestList.ChecksFailing.Render(text) }},
		{github.ChecksRunning, "● checks running", func(text string) string { return activeTheme.PullRequestList.ChecksRunning.Render(text) }},
		{github.ChecksNone, "", nil},
	}
	for _, testCase := range tests {
		test.Run(string(testCase.state), func(test *testing.T) {
			harness := newHarness(test, 160, 24)
			detail := testDetail()
			detail.Checks = testCase.state
			harness.client.SetPullRequest(detail)
			harness.run(harness.v.Init())
			out := harness.render()
			if testCase.style == nil {
				if strings.Contains(ansi.Strip(out), "checks") {
					test.Error("header mentions checks when there are none")
				}
				return
			}
			if !strings.Contains(out, testCase.style(testCase.text)) {
				test.Errorf("header lacks styled %q:\n%s", testCase.text, ansi.Strip(out))
			}
		})
	}
}
