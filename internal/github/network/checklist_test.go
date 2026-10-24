package network

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestMapDetailChecks(test *testing.T) {
	bytes, err := os.ReadFile("testdata/PullRequestChecks.json")
	if err != nil {
		test.Fatal(err)
	}
	var response struct {
		Data struct {
			Repository struct{ PullRequest graphqlPullRequestDetail }
		}
	}
	if err := json.Unmarshal(bytes, &response); err != nil {
		test.Fatal(err)
	}
	node := response.Data.Repository.PullRequest
	node.Repository.NameWithOwner = "MikePors/strigidae"
	detail, err := mapDetail(node, nil, nil, nil)
	if err != nil {
		test.Fatal(err)
	}
	want := []github.Check{
		{Name: "CI / check", Result: github.CheckPassed, URL: "https://github.com/MikePors/strigidae/actions/runs/1/job/2"},
		{Name: "CI / vulnerabilities", Result: github.CheckFailed, URL: "https://github.com/MikePors/strigidae/actions/runs/1/job/3"},
		{Name: "deploy", Result: github.CheckRunning},
		{Name: "lint", Result: github.CheckQueued, URL: "https://example.test/lint"},
		{Name: "docs", Result: github.CheckSkipped, URL: "https://example.test/docs"},
		{Name: "ci/legacy", Result: github.CheckFailed, URL: "https://ci.example.test/legacy"},
		{Name: "ci/pending", Result: github.CheckRunning},
	}
	if diff := cmp.Diff(want, detail.CheckList); diff != "" {
		test.Errorf("checks (-want +got):\n%s", diff)
	}
	if detail.CheckTotal != 130 {
		test.Errorf("CheckTotal = %d, want 130", detail.CheckTotal)
	}
	if detail.MergeState != github.MergeStateConflicts {
		test.Errorf("MergeState = %q", detail.MergeState)
	}
}

func TestCheckResultMapping(test *testing.T) {
	tests := []struct {
		status, conclusion string
		want               github.CheckResult
	}{
		{"COMPLETED", "CANCELLED", github.CheckCancelled},
		{"COMPLETED", "TIMED_OUT", github.CheckTimedOut},
		{"COMPLETED", "ACTION_REQUIRED", github.CheckActionRequired},
		{"COMPLETED", "NEUTRAL", github.CheckNeutral},
		{"COMPLETED", "STALE", github.CheckNeutral},
		{"WAITING", "", github.CheckQueued},
		{"IN_PROGRESS", "", github.CheckRunning},
	}
	for _, testCase := range tests {
		if got := github.CheckResultFromRun(testCase.status, testCase.conclusion); got != testCase.want {
			test.Errorf("%s/%s = %q, want %q", testCase.status, testCase.conclusion, got, testCase.want)
		}
	}
	states := map[string]github.MergeState{
		"DIRTY": github.MergeStateConflicts, "BEHIND": github.MergeStateBehind, "BLOCKED": github.MergeStateBlocked,
		"CLEAN": "", "UNSTABLE": "", "DRAFT": "", "UNKNOWN": "", "HAS_HOOKS": "",
	}
	for status, want := range states {
		if got := github.MergeStateFromStatus(status); got != want {
			test.Errorf("merge state %s = %q, want %q", status, got, want)
		}
	}
}
