package network

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestUpdatePullRequest(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.UpdatePullRequestInput{Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample01", Title: "New title", Body: "New\n\nbody"}
	if err := client.UpdatePullRequest(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	call := testServer.Calls()[0]
	if call.Operation != operationUpdatePullRequest {
		test.Fatalf("operation = %q", call.Operation)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample01", "title": "New title", "body": "New\n\nbody"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}
