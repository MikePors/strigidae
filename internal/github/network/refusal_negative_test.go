package network

import (
	"context"
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

// An UNPROCESSABLE message that mentions your own pull request but is not an
// approval or a request for changes must keep GitHub's message.
func TestSubmitReviewOtherOwnPullRequestRefusalIsNotMapped(test *testing.T) {
	payload := `{"data":null,"errors":[{"type":"UNPROCESSABLE","message":"Can not merge your own pull request"}]}`
	client := newTestClient(test, &server{override: respondWith(payload)})
	err := client.SubmitReview(context.Background(), github.SubmitReviewInput{PullRequestIdentifier: "PR_1", Event: github.ReviewEventComment})
	if err == nil {
		test.Fatal("no error")
	}
	if errors.Is(err, github.ErrOwnPullRequest) {
		test.Errorf("mapped to ErrOwnPullRequest: %v", err)
	}
}
