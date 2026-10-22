package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestGetMergeStatus(t *testing.T) {
	c := newTestClient(t, &server{})
	got, err := c.GetMergeStatus(context.Background(), repo, 27)
	if err != nil {
		t.Fatal(err)
	}
	want := github.MergeStatus{
		PullRequestID: "PR_kwDOExample27", HeadSHA: "0123456789abcdef0123456789abcdef01234567", BaseRef: "main",
		Allowed:             []github.MergeMethod{github.MergeMethodSquash, github.MergeMethodRebase},
		DeleteBranchOnMerge: true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestGetMergeStatusNotFound(t *testing.T) {
	s := &server{override: func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":null}}}`))
		return true
	}}
	_, err := newTestClient(t, s).GetMergeStatus(context.Background(), repo, 27)
	if !errors.Is(err, github.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMergePullRequest(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	in := github.MergeInput{Repo: repo, Number: 27, PullRequestID: "PR_kwDOExample27", Method: github.MergeMethodSquash, ExpectedHeadOID: "abc123"}
	if err := c.MergePullRequest(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	call := s.Calls()[0]
	if call.Op != opMerge {
		t.Fatalf("op = %q", call.Op)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "mergeMethod": "SQUASH", "expectedHeadOid": "abc123"}
	if diff := cmp.Diff(want, call.Vars); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestMergePullRequestRefused(t *testing.T) {
	s := &server{override: func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Required status check is expected."}]}`))
		return true
	}}
	in := github.MergeInput{Repo: repo, Number: 27, PullRequestID: "PR_1", Method: github.MergeMethodMerge, ExpectedHeadOID: "abc"}
	err := newTestClient(t, s).MergePullRequest(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "Required status check is expected.") {
		t.Errorf("err = %v, want GitHub's message", err)
	}
}

func TestMergePullRequestRefusesIncompleteInput(t *testing.T) {
	good := github.MergeInput{Repo: repo, Number: 27, PullRequestID: "PR_1", Method: github.MergeMethodMerge, ExpectedHeadOID: "abc"}
	tests := map[string]func(*github.MergeInput){
		"no pull request ID": func(in *github.MergeInput) { in.PullRequestID = "" },
		"no method":          func(in *github.MergeInput) { in.Method = "" },
		"no expected head":   func(in *github.MergeInput) { in.ExpectedHeadOID = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s := &server{}
			in := good
			mutate(&in)
			if err := newTestClient(t, s).MergePullRequest(context.Background(), in); err == nil {
				t.Error("no error")
			}
			if len(s.Calls()) != 0 {
				t.Error("a request was sent")
			}
		})
	}
}
