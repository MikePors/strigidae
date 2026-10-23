package workspace

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// flowEnv is a ready environment for a flow working against the fake, with
// no workspace involved.
func flowEnvironment(client github.Client) reviewEnvironment {
	return reviewEnvironment{
		client: client, repository: testRepository, number: 7, theme: theme.Default(),
		edit:   func(string, func(string, error) tea.Msg) tea.Cmd { return nil },
		reload: func() tea.Cmd { return nil },
		loaded: true, diffReady: true, diffHead: "h1",
	}
}

func TestReviewFlowBlocked(test *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reviewFlow, *reviewEnvironment)
		want   string // "" means not blocked
	}{
		{"ready", func(*reviewFlow, *reviewEnvironment) {}, ""},
		{"submitting", func(flow *reviewFlow, _ *reviewEnvironment) { flow.submitting = true }, "a review is being submitted"},
		{"not loaded", func(_ *reviewFlow, environment *reviewEnvironment) { environment.loaded = false }, "pull request not loaded yet"},
		{"diff not ready", func(_ *reviewFlow, environment *reviewEnvironment) { environment.diffReady = false }, "diff not loaded"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var flow reviewFlow
			environment := flowEnvironment(fake.New())
			testCase.mutate(&flow, &environment)
			command := flow.blocked(environment)
			if testCase.want == "" {
				if command != nil {
					test.Fatalf("blocked: %#v", command())
				}
				return
			}
			if diff := cmp.Diff(tea.Msg(userinterface.InfoMessage{Text: testCase.want}), command()); diff != "" {
				test.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestReviewFlowIgnoresOtherMessagesAndFlows(test *testing.T) {
	var flow, other reviewFlow
	environment := flowEnvironment(fake.New())
	if _, handled := flow.update(tea.WindowSizeMsg{}, environment); handled {
		test.Error("an unrelated message was claimed")
	}
	// A result of another flow is consumed and does nothing.
	command, handled := flow.update(reviewSubmittedMessage{flow: &other, repository: testRepository, number: 7, event: github.ReviewEventApprove}, environment)
	if !handled || command != nil {
		test.Errorf("foreign result: handled=%v cmd=%v", handled, command != nil)
	}
	// So does a result of an older attempt.
	flow.sequence = 2
	command, handled = flow.update(reviewPendingMessage{flow: &flow, repository: testRepository, number: 7, sequence: 1}, environment)
	if !handled || command != nil {
		test.Errorf("stale result: handled=%v cmd=%v", handled, command != nil)
	}
}

func TestReviewFlowDraftSurvivesEmptyEdit(test *testing.T) {
	client := fake.New()
	flow := reviewFlow{draft: "keep me", sequence: 1}
	environment := flowEnvironment(client)
	command, handled := flow.update(reviewEditedMessage{flow: &flow, repository: testRepository, number: 7, sequence: 1, event: github.ReviewEventApprove, body: " \n"}, environment)
	if !handled || command == nil || flow.draft != "keep me" {
		test.Fatalf("handled=%v cmd=%v draft=%q", handled, command != nil, flow.draft)
	}
	// The next step is the fresh status query, and nothing was sent.
	if _, ok := command().(reviewFreshMessage); !ok {
		test.Error("the edit did not lead to the fresh status query")
	}
	if len(client.Submissions()) != 0 {
		test.Error("something was sent")
	}
}
