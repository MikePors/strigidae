package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

var repo = github.Repo{Owner: "o", Name: "r"}

func seeded() *fake.Client {
	c := fake.New()
	c.Now = func() time.Time { return time.Unix(100, 0) }
	var prs []github.PullRequest
	for i := 1; i <= 5; i++ {
		prs = append(prs, github.PullRequest{Repo: repo, Number: i})
	}
	c.SetPullRequests(repo, prs)
	c.SetPullRequest(github.PullRequestDetail{PullRequest: github.PullRequest{Repo: repo, Number: 1}})
	c.SetDiff(repo, 1, "diff")
	c.SetReviewThreads(repo, 1, []github.ReviewThread{{ID: "t1"}, {ID: "t2"}})
	return c
}

func TestPagination(t *testing.T) {
	c := seeded()
	ctx := context.Background()
	var numbers []int
	var req github.PageRequest
	req.Limit = 2
	pages := 0
	for {
		p, err := c.ListPullRequests(ctx, repo, req)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, pr := range p.Items {
			numbers = append(numbers, pr.Number)
		}
		if !p.HasMore() {
			break
		}
		req.After = p.Next
	}
	if diff := cmp.Diff([]int{1, 2, 3, 4, 5}, numbers); diff != "" {
		t.Errorf("numbers mismatch:\n%s", diff)
	}
	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
	if _, err := c.ListPullRequests(ctx, repo, github.PageRequest{After: "bogus"}); err == nil {
		t.Error("want error for bad cursor")
	}
}

func TestReplyAndCommentAreRecorded(t *testing.T) {
	c := seeded()
	ctx := context.Background()

	rc, err := c.ReplyToThread(ctx, "t2", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if rc.Author != "me" || rc.Body != "hello" || !rc.CreatedAt.Equal(time.Unix(100, 0)) {
		t.Errorf("unexpected reply: %+v", rc)
	}
	if diff := cmp.Diff([]fake.Reply{{ThreadID: "t2", Body: "hello"}}, c.Replies()); diff != "" {
		t.Errorf("replies mismatch:\n%s", diff)
	}
	threads, err := c.ListReviewThreads(ctx, repo, 1, github.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(threads.Items[1].Comments); got != 1 {
		t.Errorf("reply not visible in thread: %d comments", got)
	}

	if _, err := c.AddComment(ctx, repo, 1, "top"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]fake.Comment{{Repo: repo, Number: 1, Body: "top"}}, c.Comments()); diff != "" {
		t.Errorf("comments mismatch:\n%s", diff)
	}
	d, err := c.GetPullRequest(ctx, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Comments) != 1 || d.CommentCount != 1 {
		t.Errorf("comment not visible in detail: %+v", d)
	}
}

func TestErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	t.Run("per method", func(t *testing.T) {
		c := seeded()
		c.SetError(fake.MethodGetDiff, boom)
		if _, err := c.GetDiff(ctx, repo, 1); !errors.Is(err, boom) {
			t.Errorf("want boom, got %v", err)
		}
		if _, err := c.ListPullRequests(ctx, repo, github.PageRequest{}); err != nil {
			t.Errorf("other methods should work: %v", err)
		}
		c.SetError(fake.MethodGetDiff, nil)
		if _, err := c.GetDiff(ctx, repo, 1); err != nil {
			t.Errorf("error should be cleared: %v", err)
		}
	})

	t.Run("all methods", func(t *testing.T) {
		c := seeded()
		c.SetError("", &github.RateLimitError{})
		_, err := c.ListReviewThreads(ctx, repo, 1, github.PageRequest{})
		var rl *github.RateLimitError
		if !errors.As(err, &rl) {
			t.Errorf("want RateLimitError, got %v", err)
		}
		if _, err := c.ReplyToThread(ctx, "t1", "x"); err == nil {
			t.Error("want error")
		}
		if len(c.Replies()) != 0 {
			t.Error("failed reply must not be recorded")
		}
	})

	t.Run("not found", func(t *testing.T) {
		c := seeded()
		if _, err := c.GetPullRequest(ctx, repo, 99); !errors.Is(err, github.ErrNotFound) {
			t.Errorf("want ErrNotFound, got %v", err)
		}
		if _, err := c.ReplyToThread(ctx, "nope", "x"); !errors.Is(err, github.ErrNotFound) {
			t.Errorf("want ErrNotFound, got %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		c := seeded()
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := c.GetDiff(cctx, repo, 1); !errors.Is(err, context.Canceled) {
			t.Errorf("want context.Canceled, got %v", err)
		}
	})
}

func TestCalls(t *testing.T) {
	c := seeded()
	ctx := context.Background()
	_, _ = c.GetDiff(ctx, repo, 1)
	_, _ = c.SearchPullRequests(ctx, "q", github.PageRequest{})
	want := []string{fake.MethodGetDiff, fake.MethodSearchPullRequests}
	if diff := cmp.Diff(want, c.Calls()); diff != "" {
		t.Errorf("calls mismatch:\n%s", diff)
	}
}
