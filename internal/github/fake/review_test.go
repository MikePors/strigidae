package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestSubmitReview(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		pending *github.PendingReview
		in      github.SubmitReviewInput
		wantErr error
		bad     bool
	}{
		{name: "approve without body", in: github.SubmitReviewInput{Event: github.ReviewEventApprove}},
		{name: "comment without body", in: github.SubmitReviewInput{Event: github.ReviewEventComment}, bad: true},
		{name: "request changes with body", in: github.SubmitReviewInput{Event: github.ReviewEventRequestChanges, Body: "no"}},
		{
			name:    "pending with inline comments may have no body",
			pending: &github.PendingReview{ID: "p1", CommentCount: 2},
			in:      github.SubmitReviewInput{Event: github.ReviewEventComment, PendingReviewID: "p1"},
		},
		{name: "pending gone", in: github.SubmitReviewInput{Event: github.ReviewEventApprove, PendingReviewID: "p1"}, wantErr: github.ErrNotFound},
		{name: "unknown event", in: github.SubmitReviewInput{Event: "DISMISS"}, bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := seeded()
			c.SetPendingReview(repo, 1, tt.pending)
			tt.in.Repo, tt.in.Number = repo, 1
			err := c.SubmitReview(ctx, tt.in)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.bad:
				if err == nil {
					t.Fatal("want error")
				}
			case err != nil:
				t.Fatal(err)
			}
			wantSubs := 1
			if err != nil {
				wantSubs = 0
			}
			if got := len(c.Submissions()); got != wantSubs {
				t.Errorf("submissions = %d, want %d", got, wantSubs)
			}
		})
	}
}

func TestGetReviewStatus(t *testing.T) {
	c := seeded()
	c.SetHeadSHA(repo, 1, "abc")
	c.SetPendingReview(repo, 1, &github.PendingReview{ID: "p1", Body: "b", CommentCount: 3})
	got, err := c.GetReviewStatus(context.Background(), repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := github.ReviewStatus{HeadSHA: "abc", Pending: &github.PendingReview{ID: "p1", Body: "b", CommentCount: 3}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("status (-want +got):\n%s", diff)
	}
	in := github.SubmitReviewInput{Repo: repo, Number: 1, Event: github.ReviewEventApprove, PendingReviewID: "p1"}
	if err := c.SubmitReview(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.GetReviewStatus(context.Background(), repo, 1); got.Pending != nil {
		t.Error("pending review survived its submission")
	}
}
