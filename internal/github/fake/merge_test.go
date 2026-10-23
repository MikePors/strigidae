package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func mergeFake() (*Client, github.Repository) {
	client := New()
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client.SetPullRequest(github.PullRequestDetail{
		PullRequest: github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1, State: github.PullRequestStateOpen, BaseBranch: "main", HeadSHA: "h1"},
	})
	return client, repository
}

func TestMergeStatusDefaultsAndSettings(test *testing.T) {
	client, repository := mergeFake()
	status, err := client.GetMergeStatus(context.Background(), repository, 1)
	if err != nil {
		test.Fatal(err)
	}
	want := github.MergeStatus{PullRequestIdentifier: "PR_1", HeadSHA: "h1", BaseBranch: "main", Allowed: github.MergeMethods}
	if diff := cmp.Diff(want, status); diff != "" {
		test.Errorf("default (-want +got):\n%s", diff)
	}
	client.SetMergeSettings(repository, MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodRebase}, DeleteBranchOnMerge: true})
	status, _ = client.GetMergeStatus(context.Background(), repository, 1)
	if !status.DeleteBranchOnMerge || !status.Allows(github.MergeMethodRebase) || status.Allows(github.MergeMethodSquash) {
		test.Errorf("settings not applied: %+v", status)
	}
	if _, err := client.GetMergeStatus(context.Background(), repository, 99); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("unknown PR: %v", err)
	}
}

func TestMergePullRequest(test *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Client, github.Repository, *github.MergeInput)
		fail   bool
	}{
		{"ok", func(*Client, github.Repository, *github.MergeInput) {}, false},
		{"wrong id", func(_ *Client, _ github.Repository, in *github.MergeInput) { in.PullRequestIdentifier = "x" }, true},
		{"head moved", func(client *Client, repository github.Repository, _ *github.MergeInput) {
			client.SetHeadSHA(repository, 1, "h2")
		}, true},
		{"method not allowed", func(client *Client, repository github.Repository, _ *github.MergeInput) {
			client.SetMergeSettings(repository, MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodMerge}})
		}, true},
		{"injected refusal", func(client *Client, _ github.Repository, _ *github.MergeInput) {
			client.SetError(MethodMergePullRequest, errors.New("checks failing"))
		}, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			client, repository := mergeFake()
			in := github.MergeInput{Repository: repository, PullRequestIdentifier: "PR_1", Number: 1, Method: github.MergeMethodSquash, ExpectedHeadObjectIdentifier: "h1"}
			testCase.mutate(client, repository, &in)
			err := client.MergePullRequest(context.Background(), in)
			if (err != nil) != testCase.fail {
				test.Fatalf("err = %v, fail = %v", err, testCase.fail)
			}
			wantMerges := 1
			if testCase.fail {
				wantMerges = 0
			}
			if got := len(client.Merges()); got != wantMerges {
				test.Errorf("recorded merges = %d, want %d", got, wantMerges)
			}
		})
	}
}
