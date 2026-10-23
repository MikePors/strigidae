package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestHeaderChecks(t *testing.T) {
	th := theme.New(true)
	tests := []struct {
		state github.CheckState
		text  string
		style func(string) string
	}{
		{github.ChecksPassing, "✓ checks passing", func(s string) string { return th.PullRequestList.ChecksPassing.Render(s) }},
		{github.ChecksFailing, "✗ checks failing", func(s string) string { return th.PullRequestList.ChecksFailing.Render(s) }},
		{github.ChecksRunning, "● checks running", func(s string) string { return th.PullRequestList.ChecksRunning.Render(s) }},
		{github.ChecksNone, "", nil},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			h := newHarness(t, 160, 24)
			d := testDetail()
			d.Checks = tt.state
			h.client.SetPullRequest(d)
			h.run(h.v.Init())
			out := h.render()
			if tt.style == nil {
				if strings.Contains(ansi.Strip(out), "checks") {
					t.Error("header mentions checks when there are none")
				}
				return
			}
			if !strings.Contains(out, tt.style(tt.text)) {
				t.Errorf("header lacks styled %q:\n%s", tt.text, ansi.Strip(out))
			}
		})
	}
}
