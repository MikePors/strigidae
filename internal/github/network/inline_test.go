package network

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

// The fixtures for these calls are written by hand from GitHub's schema
// (checked by introspection): agents never run mutations against GitHub.

func TestStartReview(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	got, err := client.StartReview(context.Background(), github.StartReviewInput{Repository: repository, Number: 27, PullRequestIdentifier: "PR_kwDOExample27", CommitObjectIdentifier: "abc123"})
	if err != nil {
		test.Fatal(err)
	}
	if want := (github.PendingReview{Identifier: "PRR_kwDOExample11"}); !cmp.Equal(got, want) {
		test.Errorf("review = %+v, want %+v", got, want)
	}
	want := map[string]any{"pullRequestId": "PR_kwDOExample27", "commitOID": "abc123"}
	if diff := cmp.Diff(want, testServer.Calls()[0].Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestAddReviewThread(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.AddThreadInput{
		Repository: repository, Number: 27, PendingReviewIdentifier: "PRR_kwDOExample11", Path: "internal/github/client.go",
		Line: 12, Side: github.SideRight, StartLine: 10, StartSide: github.SideRight, Body: "Consider a clearer name.",
	}
	got, err := client.AddReviewThread(context.Background(), in)
	if err != nil {
		test.Fatal(err)
	}
	if got.Identifier != "PRRT_kwDOExample20" || got.Line != 12 || got.StartLine != 10 || len(got.Comments) != 1 || !got.Comments[0].Pending {
		test.Errorf("thread = %+v", got)
	}
	want := map[string]any{
		"reviewId": "PRR_kwDOExample11", "path": "internal/github/client.go", "body": "Consider a clearer name.",
		"line": float64(12), "side": "RIGHT", "startLine": float64(10), "startSide": "RIGHT",
	}
	if diff := cmp.Diff(want, testServer.Calls()[0].Variables); diff != "" {
		test.Errorf("variables (-want +got):\n%s", diff)
	}
}

func TestAddReviewThreadSingleLine(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	in := github.AddThreadInput{Repository: repository, Number: 27, PendingReviewIdentifier: "R", Path: "a.go", Line: 3, Side: github.SideLeft, Body: "x"}
	if _, err := client.AddReviewThread(context.Background(), in); err != nil {
		test.Fatal(err)
	}
	variables := testServer.Calls()[0].Variables
	if _, ok := variables["startLine"]; ok {
		test.Errorf("single-line comment sent startLine: %v", variables)
	}
	if variables["side"] != "LEFT" {
		test.Errorf("side = %v", variables["side"])
	}
}

func TestUpdateAndDeleteReviewComment(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	got, err := client.UpdateReviewComment(context.Background(), "PRRC_kwDOExample21", "Edited.")
	if err != nil {
		test.Fatal(err)
	}
	if got.Identifier != "PRRC_kwDOExample21" || got.Body != "Edited." || !got.Pending {
		test.Errorf("comment = %+v", got)
	}
	if err := client.DeleteReviewComment(context.Background(), "PRRC_kwDOExample21"); err != nil {
		test.Fatal(err)
	}
	calls := testServer.Calls()
	if calls[0].Operation != operationUpdateReviewComment || calls[1].Operation != operationDeleteReviewComment {
		test.Errorf("ops = %q, %q", calls[0].Operation, calls[1].Operation)
	}
	if calls[1].Variables["commentId"] != "PRRC_kwDOExample21" {
		test.Errorf("vars = %v", calls[1].Variables)
	}
}

func TestGetReviewComment(test *testing.T) {
	testServer := &server{}
	client := newTestClient(test, testServer)
	got, err := client.GetReviewComment(context.Background(), "PRRC_kwDOExample21")
	if err != nil {
		test.Fatal(err)
	}
	if got.Pending || !got.ViewerCanUpdate || got.ViewerCanDelete || !got.ViewerDidAuthor || got.Body != "Consider a clearer name." {
		test.Errorf("comment = %+v", got)
	}
	if testServer.Calls()[0].Variables["id"] != "PRRC_kwDOExample21" {
		test.Errorf("vars = %v", testServer.Calls()[0].Variables)
	}
}

func TestGetReviewCommentGone(test *testing.T) {
	testServer := &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"node":null}}`))
		return true
	}}
	_, err := newTestClient(test, testServer).GetReviewComment(context.Background(), "gone")
	if !errors.Is(err, github.ErrNotFound) {
		test.Errorf("err = %v, want ErrNotFound", err)
	}
}

// jsonServer answers every request with body.
func jsonServer(body string) *server {
	return &server{override: func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
		return true
	}}
}

func TestNullPayloads(test *testing.T) {
	ctx := context.Background()
	test.Run("a thread GitHub did not create is rejected, naming the line", func(test *testing.T) {
		client := newTestClient(test, jsonServer(`{"data":{"addPullRequestReviewThread":{"thread":null}}}`))
		_, err := client.AddReviewThread(ctx, github.AddThreadInput{Repository: repository, Number: 27, PendingReviewIdentifier: "R", Path: "a/b.go", Line: 7, Side: github.SideRight, Body: "x"})
		if !errors.Is(err, github.ErrCommentRejected) || errors.Is(err, github.ErrNotFound) {
			test.Fatalf("err = %v, want ErrCommentRejected", err)
		}
		if !strings.Contains(err.Error(), "a/b.go:7") {
			test.Errorf("error does not name the line: %v", err)
		}
	})
	test.Run("a null thread mutation payload", func(test *testing.T) {
		client := newTestClient(test, jsonServer(`{"data":{"addPullRequestReviewThread":null}}`))
		_, err := client.AddReviewThread(ctx, github.AddThreadInput{Repository: repository, Number: 27, Path: "a.go", Line: 1, Side: github.SideRight})
		if !errors.Is(err, github.ErrCommentRejected) {
			test.Errorf("err = %v", err)
		}
	})
	test.Run("no review is started", func(test *testing.T) {
		client := newTestClient(test, jsonServer(`{"data":{"addPullRequestReview":{"pullRequestReview":null}}}`))
		_, err := client.StartReview(ctx, github.StartReviewInput{Repository: repository, Number: 27})
		if !errors.Is(err, github.ErrNotFound) {
			test.Errorf("err = %v", err)
		}
		client = newTestClient(test, jsonServer(`{"data":{"addPullRequestReview":null}}`))
		if _, err := client.StartReview(ctx, github.StartReviewInput{Repository: repository, Number: 27}); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("null payload: err = %v", err)
		}
	})
	test.Run("a node of another type is not a comment", func(test *testing.T) {
		client := newTestClient(test, jsonServer(`{"data":{"node":{}}}`))
		if _, err := client.GetReviewComment(ctx, "x"); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("err = %v", err)
		}
	})
	test.Run("a null update payload", func(test *testing.T) {
		client := newTestClient(test, jsonServer(`{"data":{"updatePullRequestReviewComment":null}}`))
		if _, err := client.UpdateReviewComment(ctx, "x", "y"); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("err = %v", err)
		}
	})
}
