package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func mergeFake() (*Client, github.Repo) {
	c := New()
	repo := github.Repo{Owner: "acme", Name: "widgets"}
	c.SetPullRequest(github.PullRequestDetail{
		PullRequest: github.PullRequest{ID: "PR_1", Repo: repo, Number: 1, State: github.PRStateOpen, BaseRef: "main", HeadSHA: "h1"},
	})
	return c, repo
}

func TestMergeStatusDefaultsAndSettings(t *testing.T) {
	c, repo := mergeFake()
	st, err := c.GetMergeStatus(context.Background(), repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := github.MergeStatus{PullRequestID: "PR_1", HeadSHA: "h1", BaseRef: "main", Allowed: github.MergeMethods}
	if diff := cmp.Diff(want, st); diff != "" {
		t.Errorf("default (-want +got):\n%s", diff)
	}
	c.SetMergeSettings(repo, MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodRebase}, DeleteBranchOnMerge: true})
	st, _ = c.GetMergeStatus(context.Background(), repo, 1)
	if !st.DeleteBranchOnMerge || !st.Allows(github.MergeMethodRebase) || st.Allows(github.MergeMethodSquash) {
		t.Errorf("settings not applied: %+v", st)
	}
	if _, err := c.GetMergeStatus(context.Background(), repo, 99); !errors.Is(err, github.ErrNotFound) {
		t.Errorf("unknown PR: %v", err)
	}
}

func TestMergePullRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Client, github.Repo, *github.MergeInput)
		fail   bool
	}{
		{"ok", func(*Client, github.Repo, *github.MergeInput) {}, false},
		{"no expected head", func(_ *Client, _ github.Repo, in *github.MergeInput) { in.ExpectedHeadOID = "" }, true},
		{"wrong id", func(_ *Client, _ github.Repo, in *github.MergeInput) { in.PullRequestID = "x" }, true},
		{"head moved", func(c *Client, r github.Repo, _ *github.MergeInput) { c.SetHeadSHA(r, 1, "h2") }, true},
		{"method not allowed", func(c *Client, r github.Repo, _ *github.MergeInput) {
			c.SetMergeSettings(r, MergeSettings{Allowed: []github.MergeMethod{github.MergeMethodMerge}})
		}, true},
		{"injected refusal", func(c *Client, _ github.Repo, _ *github.MergeInput) {
			c.SetError(MethodMergePullRequest, errors.New("checks failing"))
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, repo := mergeFake()
			in := github.MergeInput{Repo: repo, PullRequestID: "PR_1", Number: 1, Method: github.MergeMethodSquash, ExpectedHeadOID: "h1"}
			tt.mutate(c, repo, &in)
			err := c.MergePullRequest(context.Background(), in)
			if (err != nil) != tt.fail {
				t.Fatalf("err = %v, fail = %v", err, tt.fail)
			}
			wantMerges := 1
			if tt.fail {
				wantMerges = 0
			}
			if got := len(c.Merges()); got != wantMerges {
				t.Errorf("recorded merges = %d, want %d", got, wantMerges)
			}
		})
	}
}
