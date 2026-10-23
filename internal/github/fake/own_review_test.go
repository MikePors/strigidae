package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func TestSubmitReviewOnOwnPullRequest(test *testing.T) {
	tests := []struct {
		name    string
		event   github.ReviewEvent
		wantOwn bool
	}{
		{"approve is refused", github.ReviewEventApprove, true},
		{"request changes is refused", github.ReviewEventRequestChanges, true},
		{"comment is accepted", github.ReviewEventComment, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			client := seeded()
			client.SetPullRequest(github.PullRequestDetail{
				PullRequest:     github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1},
				ViewerDidAuthor: true,
			})
			err := client.SubmitReview(context.Background(), github.SubmitReviewInput{
				Repository: repository, Number: 1, PullRequestIdentifier: "PR_1", Event: testCase.event, Body: "text",
			})
			if testCase.wantOwn {
				if !errors.Is(err, github.ErrOwnPullRequest) {
					test.Fatalf("err = %v, want ErrOwnPullRequest", err)
				}
				if len(client.Submissions()) != 0 {
					test.Error("a refused review was recorded")
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if len(client.Submissions()) != 1 {
				test.Errorf("submissions = %d, want 1", len(client.Submissions()))
			}
		})
	}
}
