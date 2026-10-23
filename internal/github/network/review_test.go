package network

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestGetReviewStatus(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.GetReviewStatus(context.Background(), repository, 27)
	if err != nil {
		t.Fatal(err)
	}
	want := github.ReviewStatus{
		PullRequestIdentifier: "PR_kwDOExample27",
		HeadSHA:               "0123456789abcdef0123456789abcdef01234567",
		Pending:               &github.PendingReview{Identifier: "PRR_kwDOExample09", Body: "Draft summary", CommentCount: 3, CommitObjectIdentifier: "0123456789abcdef0123456789abcdef01234567"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewCreates(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	in := github.SubmitReviewInput{
		Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27",
		Event: github.ReviewEventApprove, Body: "LGTM", CommitObjectIdentifier: "abc123",
	}
	if err := c.SubmitReview(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	call := s.Calls()[0]
	if call.Operation != operationAddReview {
		t.Fatalf("op = %q", call.Operation)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "event": "APPROVE", "body": "LGTM", "commitOID": "abc123"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewPending(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	in := github.SubmitReviewInput{
		Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27", PendingReviewIdentifier: "PRR_kwDOExample09",
		Event: github.ReviewEventRequestChanges, Body: "fix it", CommitObjectIdentifier: "ignored",
	}
	if err := c.SubmitReview(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	call := s.Calls()[0]
	if call.Operation != operationSubmitReview {
		t.Fatalf("op = %q", call.Operation)
	}
	want := map[string]any{"reviewId": "PRR_kwDOExample09", "event": "REQUEST_CHANGES", "body": "fix it"}
	if diff := cmp.Diff(want, call.Variables); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestSubmitReviewNotFound(t *testing.T) {
	s := &server{override: func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a node"}]}`))
		return true
	}}
	c := newTestClient(t, s)
	err := c.SubmitReview(context.Background(), github.SubmitReviewInput{PendingReviewIdentifier: "gone", Event: github.ReviewEventComment})
	if !errors.Is(err, github.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
