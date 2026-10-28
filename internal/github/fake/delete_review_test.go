package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestDeleteReview(test *testing.T) {
	client := seeded()
	client.SetPendingReview(repository, 1, &github.PendingReview{Identifier: "r1", CommentCount: 1})
	client.SetReviewThreads(repository, 1, []github.ReviewThread{
		{Identifier: "draft", Comments: []github.ReviewComment{{Identifier: "c1", Pending: true}}},
		{Identifier: "kept", Comments: []github.ReviewComment{{Identifier: "c2"}, {Identifier: "c3", Pending: true}}},
	})
	ctx := context.Background()
	if err := client.DeleteReview(ctx, "nope"); !errors.Is(err, github.ErrNotFound) {
		test.Fatalf("unknown review: err = %v, want ErrNotFound", err)
	}
	if err := client.DeleteReview(ctx, "r1"); err != nil {
		test.Fatal(err)
	}
	status, err := client.GetReviewStatus(ctx, repository, 1)
	if err != nil || status.Pending != nil {
		test.Errorf("pending = %+v, err %v", status.Pending, err)
	}
	page, err := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Identifier != "kept" || len(page.Items[0].Comments) != 1 {
		test.Errorf("threads = %+v, err %v", page.Items, err)
	}
	if diff := cmp.Diff([]string{"r1"}, client.DeletedReviews()); diff != "" {
		test.Errorf("deleted (-want +got):\n%s", diff)
	}
	boom := errors.New("boom")
	client.SetError(fake.MethodDeleteReview, boom)
	if err := client.DeleteReview(ctx, "r1"); !errors.Is(err, boom) {
		test.Errorf("injected err = %v", err)
	}
}
