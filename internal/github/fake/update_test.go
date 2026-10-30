package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestUpdatePullRequest(test *testing.T) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	for _, testCase := range []struct {
		name      string
		canUpdate bool
		input     github.UpdatePullRequestInput
		wantErr   error
		wantTitle string
	}{
		{"updates", true, github.UpdatePullRequestInput{Repository: repository, Number: 1, PullRequestIdentifier: "PR_1", Title: "New", Body: "Text"}, nil, "New"},
		{"forbidden", false, github.UpdatePullRequestInput{Repository: repository, Number: 1, PullRequestIdentifier: "PR_1", Title: "New"}, github.ErrForbidden, "Old"},
		{"unknown pull request", true, github.UpdatePullRequestInput{Repository: repository, Number: 2, PullRequestIdentifier: "PR_1", Title: "New"}, github.ErrNotFound, "Old"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			client := New()
			client.SetPullRequest(github.PullRequestDetail{
				PullRequest:     github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1, Title: "Old"},
				ViewerCanUpdate: testCase.canUpdate,
			})
			err := client.UpdatePullRequest(context.Background(), testCase.input)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			detail, _ := client.GetPullRequest(context.Background(), repository, 1)
			if detail.Title != testCase.wantTitle {
				test.Errorf("title = %q, want %q", detail.Title, testCase.wantTitle)
			}
			wantUpdates := 0
			if testCase.wantErr == nil {
				wantUpdates = 1
			}
			if got := len(client.PullRequestUpdates()); got != wantUpdates {
				test.Errorf("recorded %d updates, want %d", got, wantUpdates)
			}
		})
	}
}

func TestUpdatePullRequestRecordsBothFields(test *testing.T) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client := New()
	client.SetPullRequest(github.PullRequestDetail{PullRequest: github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1}, ViewerCanUpdate: true})
	input := github.UpdatePullRequestInput{Repository: repository, Number: 1, PullRequestIdentifier: "PR_1", Title: "T", Body: "B"}
	if err := client.UpdatePullRequest(context.Background(), input); err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]github.UpdatePullRequestInput{input}, client.PullRequestUpdates()); diff != "" {
		test.Errorf("updates (-want +got):\n%s", diff)
	}
}
