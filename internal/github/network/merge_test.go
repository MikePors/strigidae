package network

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestGetMergeStatus(test *testing.T) {
	client := newTestClient(test, &server{})
	got, err := client.GetMergeStatus(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	want := github.MergeStatus{
		PullRequestIdentifier: "PR_kwDOExample27", HeadSHA: "0123456789abcdef0123456789abcdef01234567", BaseBranch: "main",
		Allowed:             []github.MergeMethod{github.MergeMethodSquash, github.MergeMethodRebase},
		DeleteBranchOnMerge: true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestGetMergeStatusNotFound(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"repository":{"pullRequest":null}}}`))
		return true
	}}
	_, err := newTestClient(test, testServer).GetMergeStatus(context.Background(), repository, 27)
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMergePullRequest(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.MergeInput{Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27", Method: github.MergeMethodSquash, ExpectedHeadObjectIdentifier: "abc123"}
	if err := client.MergePullRequest(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	call := testServer.Calls()[0]
	if call.Operation != operationMerge {
		test.Fatalf("op = %q", call.Operation)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "mergeMethod": "SQUASH", "expectedHeadOid": "abc123"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestMergePullRequestRefused(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":null,"errors":[{"message":"Required status check is expected."}]}`))
		return true
	}}
	err := newTestClient(test, testServer).MergePullRequest(context.Background(), github.MergeInput{Repository: repository, Number: 27})
	if err == nil || !strings.Contains(err.Error(), "Required status check is expected.") {
		test.Errorf("err = %v, want GitHub's message", err)
	}
}
