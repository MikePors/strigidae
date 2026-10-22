package workspace

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// flowEnv is a ready environment for a flow working against the fake, with
// no workspace involved.
func flowEnv(c github.Client) reviewEnv {
	return reviewEnv{
		client: c, repo: testRepo, number: 7, theme: theme.Default(),
		edit:   func(string, func(string, error) tea.Msg) tea.Cmd { return nil },
		reload: func() tea.Cmd { return nil },
		loaded: true, diffReady: true, diffHead: "h1",
	}
}

func TestReviewFlowBlocked(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reviewFlow, *reviewEnv)
		want   string // "" means not blocked
	}{
		{"ready", func(*reviewFlow, *reviewEnv) {}, ""},
		{"submitting", func(f *reviewFlow, _ *reviewEnv) { f.submitting = true }, "a review is being submitted"},
		{"not loaded", func(_ *reviewFlow, e *reviewEnv) { e.loaded = false }, "pull request not loaded yet"},
		{"diff not ready", func(_ *reviewFlow, e *reviewEnv) { e.diffReady = false }, "diff not loaded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f reviewFlow
			env := flowEnv(fake.New())
			tt.mutate(&f, &env)
			cmd := f.blocked(env)
			if tt.want == "" {
				if cmd != nil {
					t.Fatalf("blocked: %#v", cmd())
				}
				return
			}
			if diff := cmp.Diff(tea.Msg(ui.InfoMsg{Text: tt.want}), cmd()); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestReviewFlowIgnoresOtherMessagesAndFlows(t *testing.T) {
	var f, other reviewFlow
	env := flowEnv(fake.New())
	if _, handled := f.update(tea.WindowSizeMsg{}, env); handled {
		t.Error("an unrelated message was claimed")
	}
	// A result of another flow is consumed and does nothing.
	cmd, handled := f.update(reviewSubmittedMsg{flow: &other, repo: testRepo, number: 7, event: github.ReviewEventApprove}, env)
	if !handled || cmd != nil {
		t.Errorf("foreign result: handled=%v cmd=%v", handled, cmd != nil)
	}
	// So does a result of an older attempt.
	f.seq = 2
	cmd, handled = f.update(reviewPendingMsg{flow: &f, repo: testRepo, number: 7, seq: 1}, env)
	if !handled || cmd != nil {
		t.Errorf("stale result: handled=%v cmd=%v", handled, cmd != nil)
	}
}

func TestReviewFlowDraftSurvivesEmptyEdit(t *testing.T) {
	c := fake.New()
	f := reviewFlow{draft: "keep me", seq: 1}
	env := flowEnv(c)
	cmd, handled := f.update(reviewEditedMsg{flow: &f, repo: testRepo, number: 7, seq: 1, event: github.ReviewEventApprove, body: " \n"}, env)
	if !handled || cmd == nil || f.draft != "keep me" {
		t.Fatalf("handled=%v cmd=%v draft=%q", handled, cmd != nil, f.draft)
	}
	// The next step is the fresh status query, and nothing was sent.
	if _, ok := cmd().(reviewFreshMsg); !ok {
		t.Error("the edit did not lead to the fresh status query")
	}
	if len(c.Submissions()) != 0 {
		t.Error("something was sent")
	}
}
