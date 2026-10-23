package network

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

// The first two payloads are real answers from GitHub to addPullRequestReview
// on the viewer's own pull request.
const (
	ownApprovePayload        = `{"data":{"addPullRequestReview":null},"errors":[{"type":"UNPROCESSABLE","path":["addPullRequestReview"],"locations":[{"line":1,"column":46}],"message":"Review Can not approve your own pull request"}]}`
	ownRequestChangesPayload = `{"data":{"addPullRequestReview":null},"errors":[{"type":"UNPROCESSABLE","path":["addPullRequestReview"],"locations":[{"line":1,"column":46}],"message":"Review Can not request changes on your own pull request"}]}`
)

func TestSubmitReviewRefusals(test *testing.T) {
	tests := []struct {
		name        string
		payload     string
		event       github.ReviewEvent
		wantOwn     bool
		wantMessage string
	}{
		{"approve own pull request", ownApprovePayload, github.ReviewEventApprove, true, "Review Can not approve your own pull request"},
		{"request changes on own pull request", ownRequestChangesPayload, github.ReviewEventRequestChanges, true, "Review Can not request changes on your own pull request"},
		{
			"message matched regardless of case",
			`{"data":null,"errors":[{"type":"UNPROCESSABLE","message":"CAN NOT APPROVE YOUR OWN PULL REQUEST"}]}`,
			github.ReviewEventApprove, true, "CAN NOT APPROVE YOUR OWN PULL REQUEST",
		},
		{
			"other unprocessable refusal",
			`{"data":null,"errors":[{"type":"UNPROCESSABLE","message":"Review body is too long"}]}`,
			github.ReviewEventComment, false, "Review body is too long",
		},
		{
			"own pull request wording under another type",
			`{"data":null,"errors":[{"type":"OTHER","message":"Can not approve your own pull request"}]}`,
			github.ReviewEventApprove, false, "Can not approve your own pull request",
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			client := newTestClient(test, &server{override: respondWith(testCase.payload)})
			err := client.SubmitReview(context.Background(), github.SubmitReviewInput{PullRequestIdentifier: "PR_1", Event: testCase.event})
			if err == nil {
				test.Fatal("no error")
			}
			if got := errors.Is(err, github.ErrOwnPullRequest); got != testCase.wantOwn {
				test.Errorf("errors.Is(ErrOwnPullRequest) = %v, want %v (err: %v)", got, testCase.wantOwn, err)
			}
			if !strings.Contains(err.Error(), testCase.wantMessage) {
				test.Errorf("original message lost: %v", err)
			}
		})
	}
}

func TestGetPullRequestViewerDidNotAuthor(test *testing.T) {
	body, err := os.ReadFile("testdata/PullRequestDetail.json")
	if err != nil {
		test.Fatal(err)
	}
	altered := strings.Replace(string(body), `"viewerDidAuthor": true`, `"viewerDidAuthor": false`, 1)
	client := newTestClient(test, &server{override: func(received call, writer http.ResponseWriter) bool {
		if received.Operation != operationPullRequestDetail {
			return false
		}
		return respondWith(altered)(received, writer)
	}})
	got, err := client.GetPullRequest(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	if got.ViewerDidAuthor {
		test.Error("ViewerDidAuthor = true, want false")
	}
}
