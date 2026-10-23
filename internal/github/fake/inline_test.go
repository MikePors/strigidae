package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func inlineClient() (*Client, github.Repository) {
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	client := New()
	client.SetPullRequest(github.PullRequestDetail{PullRequest: github.PullRequest{Identifier: "PR1", Repository: repository, Number: 7, HeadSHA: "abc"}})
	return client, repository
}

func TestInlineLifecycle(test *testing.T) {
	ctx := context.Background()
	client, repository := inlineClient()
	pendingReview, err := client.StartReview(ctx, github.StartReviewInput{Repository: repository, Number: 7, PullRequestIdentifier: "PR1", CommitObjectIdentifier: "abc"})
	if err != nil {
		test.Fatal(err)
	}
	if _, err := client.StartReview(ctx, github.StartReviewInput{Repository: repository, Number: 7, PullRequestIdentifier: "PR1", CommitObjectIdentifier: "abc"}); err == nil {
		test.Error("a second pending review should be refused")
	}
	thread, err := client.AddReviewThread(ctx, github.AddThreadInput{Repository: repository, Number: 7, PendingReviewIdentifier: pendingReview.Identifier, Path: "a.go", Line: 5, Side: github.SideRight, Body: "hm"})
	if err != nil {
		test.Fatal(err)
	}
	status, _ := client.GetReviewStatus(ctx, repository, 7)
	if status.Pending == nil || status.Pending.CommentCount != 1 {
		test.Fatalf("status = %+v", status.Pending)
	}
	page, _ := client.ListReviewThreads(ctx, repository, 7, github.PageRequest{})
	if len(page.Items) != 1 || !page.Items[0].Comments[0].Pending {
		test.Fatalf("threads = %+v", page.Items)
	}
	commentIdentifier := thread.Comments[0].Identifier
	got, err := client.UpdateReviewComment(ctx, commentIdentifier, "better")
	if err != nil || got.Body != "better" {
		test.Fatalf("update = %+v, %v", got, err)
	}
	if err := client.DeleteReviewComment(ctx, commentIdentifier); err != nil {
		test.Fatal(err)
	}
	page, _ = client.ListReviewThreads(ctx, repository, 7, github.PageRequest{})
	if len(page.Items) != 0 {
		test.Errorf("thread survived deleting its only comment: %+v", page.Items)
	}
	status, _ = client.GetReviewStatus(ctx, repository, 7)
	if status.Pending.CommentCount != 0 {
		test.Errorf("count = %d", status.Pending.CommentCount)
	}
}

func TestInlineErrors(test *testing.T) {
	ctx := context.Background()
	client, repository := inlineClient()
	add := func(in github.AddThreadInput) error {
		in.Repository, in.Number = repository, 7
		_, err := client.AddReviewThread(ctx, in)
		return err
	}
	if err := add(github.AddThreadInput{PendingReviewIdentifier: "nope", Path: "a", Line: 1, Side: github.SideRight, Body: "x"}); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("no pending review: %v", err)
	}
	pendingReview, _ := client.StartReview(ctx, github.StartReviewInput{Repository: repository, Number: 7, PullRequestIdentifier: "PR1", CommitObjectIdentifier: "abc"})
	if err := add(github.AddThreadInput{PendingReviewIdentifier: pendingReview.Identifier, Path: "a", Line: 1, Side: github.SideRight, Body: " "}); err == nil {
		test.Error("an empty body was accepted")
	}
	// The cases of checkRange are in TestCheckRange; this keeps its wiring tested.
	if err := add(github.AddThreadInput{PendingReviewIdentifier: pendingReview.Identifier, Path: "a", Line: 0, Side: github.SideRight, Body: "x"}); err == nil {
		test.Error("a bad range was accepted")
	}
	client.SetError(MethodAddReviewThread, errors.New("boom"))
	if err := add(github.AddThreadInput{PendingReviewIdentifier: pendingReview.Identifier, Path: "a", Line: 1, Side: github.SideRight, Body: "x"}); err == nil {
		test.Error("injected error not returned")
	}
	if _, err := client.UpdateReviewComment(ctx, "missing", "x"); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("update missing: %v", err)
	}
}

func TestCheckRange(test *testing.T) {
	lineRange, side := github.SideRight, github.SideLeft
	tests := []struct {
		name string
		in   github.AddThreadInput
		ok   bool
	}{
		{"single line", github.AddThreadInput{Path: "a", Line: 3, Side: lineRange}, true},
		{"multi-line", github.AddThreadInput{Path: "a", Line: 5, Side: side, StartLine: 3, StartSide: side}, true},
		{"one-line range", github.AddThreadInput{Path: "a", Line: 3, Side: lineRange, StartLine: 3, StartSide: lineRange}, true},
		{"no path", github.AddThreadInput{Line: 3, Side: lineRange}, false},
		{"line zero", github.AddThreadInput{Path: "a", Line: 0, Side: lineRange}, false},
		{"negative line", github.AddThreadInput{Path: "a", Line: -1, Side: lineRange}, false},
		{"unknown side", github.AddThreadInput{Path: "a", Line: 3, Side: "MIDDLE"}, false},
		{"start after line", github.AddThreadInput{Path: "a", Line: 3, Side: lineRange, StartLine: 4, StartSide: lineRange}, false},
		{"negative start", github.AddThreadInput{Path: "a", Line: 3, Side: lineRange, StartLine: -1, StartSide: lineRange}, false},
		{"range without a start side", github.AddThreadInput{Path: "a", Line: 5, Side: lineRange, StartLine: 3}, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if err := checkRange(testCase.in); (err == nil) != testCase.ok {
				test.Errorf("checkRange = %v, want ok = %v", err, testCase.ok)
			}
		})
	}
}

// The fake allows what the can-flags allow, as GitHub does: other people's
// comments too when the viewer has write access, and not the viewer's own when
// the flag is false.
func TestUpdateAndDeleteFollowTheFlags(test *testing.T) {
	ctx := context.Background()
	repository := github.Repository{Owner: "acme", Name: "widgets"}
	comments := []github.ReviewComment{
		{Identifier: "own-ok", Author: "me", ViewerCanUpdate: true, ViewerCanDelete: true},
		{Identifier: "own-no-update", Author: "me", ViewerCanDelete: true},
		{Identifier: "own-no-delete", Author: "me", ViewerCanUpdate: true},
		{Identifier: "other", Author: "bob"}, // the fake gives other people's comments the admin flags
	}
	tests := []struct {
		identifier          string
		update, removeError error
	}{
		{"own-ok", nil, nil},
		{"own-no-update", github.ErrForbidden, nil},
		{"own-no-delete", nil, github.ErrForbidden},
		{"other", nil, nil},
		{"missing", github.ErrNotFound, github.ErrNotFound},
	}
	for _, testCase := range tests {
		test.Run(testCase.identifier, func(test *testing.T) {
			client := New()
			client.SetReviewThreads(repository, 7, []github.ReviewThread{{Identifier: "t", Path: "a.go", Line: 1, Comments: comments}})
			if _, err := client.UpdateReviewComment(ctx, testCase.identifier, "new"); !errors.Is(err, testCase.update) {
				test.Errorf("update err = %v, want %v", err, testCase.update)
			}
			if err := client.DeleteReviewComment(ctx, testCase.identifier); !errors.Is(err, testCase.removeError) {
				test.Errorf("delete err = %v, want %v", err, testCase.removeError)
			}
		})
	}
}
