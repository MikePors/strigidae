package network

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestGetReviewStatus(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	got, err := client.GetReviewStatus(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	want := github.ReviewStatus{
		PullRequestIdentifier: "PR_kwDOExample27",
		HeadSHA:               "0123456789abcdef0123456789abcdef01234567",
		Pending:               &github.PendingReview{Identifier: "PRR_kwDOExample09", Body: "Draft summary", CommentCount: 3, CommitObjectIdentifier: "0123456789abcdef0123456789abcdef01234567"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewCreates(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.SubmitReviewInput{
		Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27",
		Event: github.ReviewEventApprove, Body: "LGTM", CommitObjectIdentifier: "abc123",
	}
	if err := client.SubmitReview(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	call := testServer.Calls()[0]
	if call.Operation != operationAddReview {
		test.Fatalf("op = %q", call.Operation)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "event": "APPROVE", "body": "LGTM", "commitOID": "abc123"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewPending(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.SubmitReviewInput{
		Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27", PendingReviewIdentifier: "PRR_kwDOExample09",
		Event: github.ReviewEventRequestChanges, Body: "fix it", CommitObjectIdentifier: "ignored",
	}
	if err := client.SubmitReview(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	call := testServer.Calls()[0]
	if call.Operation != operationSubmitReview {
		test.Fatalf("op = %q", call.Operation)
	}
	want := map[string]any{"reviewId": "PRR_kwDOExample09", "event": "REQUEST_CHANGES", "body": "fix it"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewNotFound(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":null,"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a node"}]}`))
		return true
	}}
	client := newTestClient(test, testServer)
	err := client.SubmitReview(context.Background(), github.SubmitReviewInput{PendingReviewIdentifier: "gone", Event: github.ReviewEventComment})
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}
