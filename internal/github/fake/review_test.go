package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestSubmitReview(test *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		pending      *github.PendingReview
		in           github.SubmitReviewInput
		wantErr      error
		bad          bool
		noIdentifier bool
	}{
		{name: "approve without body", in: github.SubmitReviewInput{Event: github.ReviewEventApprove}},
		{name: "comment without body", in: github.SubmitReviewInput{Event: github.ReviewEventComment}, bad: true},
		{name: "request changes with body", in: github.SubmitReviewInput{Event: github.ReviewEventRequestChanges, Body: "no"}},
		{
			name:    "pending with inline comments may have no body",
			pending: &github.PendingReview{Identifier: "p1", CommentCount: 2},
			in:      github.SubmitReviewInput{Event: github.ReviewEventComment, PendingReviewIdentifier: "p1"},
		},
		{name: "pending gone", in: github.SubmitReviewInput{Event: github.ReviewEventApprove, PendingReviewIdentifier: "p1"}, wantErr: github.ErrNotFound},
		{name: "empty PR ID", in: github.SubmitReviewInput{Event: github.ReviewEventApprove}, noIdentifier: true, bad: true},
		{name: "wrong PR ID", in: github.SubmitReviewInput{Event: github.ReviewEventApprove, PullRequestIdentifier: "PR_9"}, bad: true},
		{name: "unknown event", in: github.SubmitReviewInput{Event: "DISMISS"}, bad: true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			client := seeded()
			client.SetPendingReview(repository, 1, testCase.pending)
			testCase.in.Repository, testCase.in.Number = repository, 1
			if testCase.in.PullRequestIdentifier == "" && !testCase.noIdentifier {
				testCase.in.PullRequestIdentifier = "PR_1"
			}
			err := client.SubmitReview(ctx, testCase.in)
			switch {
			case testCase.wantErr != nil:
				if !errors.Is(err, testCase.wantErr) {
					test.Fatalf("err = %v, want %v", err, testCase.wantErr)
				}
			case testCase.bad:
				if err == nil {
					test.Fatal("want error")
				}
			case err != nil:
				test.Fatal(err)
			}
			wantSubs := 1
			if err != nil {
				wantSubs = 0
			}
			if got := len(client.Submissions()); got != wantSubs {
				test.Errorf("submissions = %d, want %d", got, wantSubs)
			}
		})
	}
}

func TestGetReviewStatus(test *testing.T) {
	client := seeded()
	client.SetHeadSHA(repository, 1, "abc")
	client.SetPendingReview(repository, 1, &github.PendingReview{Identifier: "p1", Body: "b", CommentCount: 3})
	got, err := client.GetReviewStatus(context.Background(), repository, 1)
	if err != nil {
		test.Fatal(err)
	}
	want := github.ReviewStatus{PullRequestIdentifier: "PR_1", HeadSHA: "abc", Pending: &github.PendingReview{Identifier: "p1", Body: "b", CommentCount: 3}}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
	in := github.SubmitReviewInput{Repository: repository, Number: 1, PullRequestIdentifier: "PR_1", Event: github.ReviewEventApprove, PendingReviewIdentifier: "p1"}
	if err := client.SubmitReview(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	if got, _ := client.GetReviewStatus(context.Background(), repository, 1); got.Pending != nil {
		test.Error("pending review survived its submission")
	}
}
