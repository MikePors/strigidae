package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

// The fixtures for these calls are written by hand from GitHub's schema
// (checked by introspection): agents never run mutations against GitHub.

func TestStartReview(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.StartReview(context.Background(), github.StartReviewInput{Repo: repo, Number: 27, PullRequestID: "PR_kwDOExample27", CommitOID: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if want := (github.PendingReview{ID: "PRR_kwDOExample11"}); !cmp.Equal(got, want) {
		t.Errorf("review = %+v, want %+v", got, want)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "commitOID": "abc123"}
	if diff := cmp.Diff(want, s.Calls()[0].Vars); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestAddReviewThread(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	in := github.AddThreadInput{
		Repo: repo, Number: 27, PendingReviewID: "PRR_kwDOExample11", Path: "internal/github/client.go",
		Line: 12, Side: github.SideRight, StartLine: 10, StartSide: github.SideRight, Body: "Consider a clearer name.",
	}
	got, err := c.AddReviewThread(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "PRRT_kwDOExample20" || got.Line != 12 || got.StartLine != 10 || len(got.Comments) != 1 || !got.Comments[0].Pending {
		t.Errorf("thread = %+v", got)
	}
	want := map[string]any{
		"reviewId": "PRR_kwDOExample11", "path": "internal/github/client.go", "body": "Consider a clearer name.",
		"line": float64(12), "side": "RIGHT", "startLine": float64(10), "startSide": "RIGHT",
	}
	if diff := cmp.Diff(want, s.Calls()[0].Vars); diff != "" {
		t.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestAddReviewThreadSingleLine(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	in := github.AddThreadInput{Repo: repo, Number: 27, PendingReviewID: "R", Path: "a.go", Line: 3, Side: github.SideLeft, Body: "x"}
	if _, err := c.AddReviewThread(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	vars := s.Calls()[0].Vars
	if _, ok := vars["startLine"]; ok {
		t.Errorf("single-line comment sent startLine: %v", vars)
	}
	if vars["side"] != "LEFT" {
		t.Errorf("side = %v", vars["side"])
	}
}

func TestUpdateAndDeleteReviewComment(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.UpdateReviewComment(context.Background(), "PRRC_kwDOExample21", "Edited.")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "PRRC_kwDOExample21" || got.Body != "Edited." || !got.Pending {
		t.Errorf("comment = %+v", got)
	}
	if err := c.DeleteReviewComment(context.Background(), "PRRC_kwDOExample21"); err != nil {
		t.Fatal(err)
	}
	calls := s.Calls()
	if calls[0].Op != opUpdateReviewComment || calls[1].Op != opDeleteReviewComment {
		t.Errorf("ops = %q, %q", calls[0].Op, calls[1].Op)
	}
	if calls[1].Vars["commentId"] != "PRRC_kwDOExample21" {
		t.Errorf("vars = %v", calls[1].Vars)
	}
}

func TestGetReviewComment(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.GetReviewComment(context.Background(), "PRRC_kwDOExample21")
	if err != nil {
		t.Fatal(err)
	}
	if got.Pending || !got.ViewerCanUpdate || got.ViewerCanDelete || got.Body != "Consider a clearer name." {
		t.Errorf("comment = %+v", got)
	}
	if s.Calls()[0].Vars["id"] != "PRRC_kwDOExample21" {
		t.Errorf("vars = %v", s.Calls()[0].Vars)
	}
}

func TestGetReviewCommentGone(t *testing.T) {
	s := &server{override: func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"node":null}}`))
		return true
	}}
	_, err := newTestClient(t, s).GetReviewComment(context.Background(), "gone")
	if !errors.Is(err, github.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
