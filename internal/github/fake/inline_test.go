package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

func inlineClient() (*Client, github.Repo) {
	repo := github.Repo{Owner: "acme", Name: "widgets"}
	c := New()
	c.SetPullRequest(github.PullRequestDetail{PullRequest: github.PullRequest{ID: "PR1", Repo: repo, Number: 7, HeadSHA: "abc"}})
	return c, repo
}

func TestInlineLifecycle(t *testing.T) {
	ctx := context.Background()
	c, repo := inlineClient()
	p, err := c.StartReview(ctx, github.StartReviewInput{Repo: repo, Number: 7, PullRequestID: "PR1", CommitOID: "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.StartReview(ctx, github.StartReviewInput{Repo: repo, Number: 7, PullRequestID: "PR1", CommitOID: "abc"}); err == nil {
		t.Error("a second pending review should be refused")
	}
	th, err := c.AddReviewThread(ctx, github.AddThreadInput{Repo: repo, Number: 7, PendingReviewID: p.ID, Path: "a.go", Line: 5, Side: github.SideRight, Body: "hm"})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := c.GetReviewStatus(ctx, repo, 7)
	if st.Pending == nil || st.Pending.CommentCount != 1 {
		t.Fatalf("status = %+v", st.Pending)
	}
	page, _ := c.ListReviewThreads(ctx, repo, 7, github.PageRequest{})
	if len(page.Items) != 1 || !page.Items[0].Comments[0].Pending {
		t.Fatalf("threads = %+v", page.Items)
	}
	cid := th.Comments[0].ID
	got, err := c.UpdateReviewComment(ctx, cid, "better")
	if err != nil || got.Body != "better" {
		t.Fatalf("update = %+v, %v", got, err)
	}
	if err := c.DeleteReviewComment(ctx, cid); err != nil {
		t.Fatal(err)
	}
	page, _ = c.ListReviewThreads(ctx, repo, 7, github.PageRequest{})
	if len(page.Items) != 0 {
		t.Errorf("thread survived deleting its only comment: %+v", page.Items)
	}
	st, _ = c.GetReviewStatus(ctx, repo, 7)
	if st.Pending.CommentCount != 0 {
		t.Errorf("count = %d", st.Pending.CommentCount)
	}
}

func TestInlineErrors(t *testing.T) {
	ctx := context.Background()
	c, repo := inlineClient()
	add := func(in github.AddThreadInput) error {
		in.Repo, in.Number = repo, 7
		_, err := c.AddReviewThread(ctx, in)
		return err
	}
	if err := add(github.AddThreadInput{PendingReviewID: "nope", Path: "a", Line: 1, Side: github.SideRight, Body: "x"}); !errors.Is(err, github.ErrNotFound) {
		t.Errorf("no pending review: %v", err)
	}
	p, _ := c.StartReview(ctx, github.StartReviewInput{Repo: repo, Number: 7, PullRequestID: "PR1", CommitOID: "abc"})
	bad := []github.AddThreadInput{
		{PendingReviewID: p.ID, Path: "a", Line: 1, Side: github.SideRight, Body: " "},
		{PendingReviewID: p.ID, Path: "a", Line: 0, Side: github.SideRight, Body: "x"},
		{PendingReviewID: p.ID, Path: "a", Line: 3, Side: github.SideRight, StartLine: 4, StartSide: github.SideRight, Body: "x"},
	}
	for i, in := range bad {
		if err := add(in); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
	c.SetError(MethodAddReviewThread, errors.New("boom"))
	if err := add(github.AddThreadInput{PendingReviewID: p.ID, Path: "a", Line: 1, Side: github.SideRight, Body: "x"}); err == nil {
		t.Error("injected error not returned")
	}
	if _, err := c.UpdateReviewComment(ctx, "missing", "x"); !errors.Is(err, github.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
}
