package fake_test

import (
	"context"
	"errors"
	"sync"
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
		prs = append(prs, github.PullRequest{Repo: repo, Number: i, State: github.PRStateOpen, UpdatedAt: time.Unix(int64(1000-i), 0)})
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
	_, _ = c.SearchPullRequests(ctx, "q", github.PageRequest{After: "3", Limit: 5})
	want := []fake.Call{
		{Method: fake.MethodGetDiff, Repo: repo, Number: 1},
		{Method: fake.MethodSearchPullRequests, Query: "q", Page: github.PageRequest{After: "3", Limit: 5}},
	}
	if diff := cmp.Diff(want, c.Calls()); diff != "" {
		t.Errorf("calls mismatch:\n%s", diff)
	}
}

func TestSearchIsKeyedByQuery(t *testing.T) {
	c := seeded()
	ctx := context.Background()
	c.SetSearchResults("review-requested:@me", []github.PullRequest{{Repo: repo, Number: 7}})
	got, err := c.SearchPullRequests(ctx, "review-requested:@me", github.PageRequest{})
	if err != nil || len(got.Items) != 1 || got.Items[0].Number != 7 {
		t.Fatalf("seeded query: %+v, %v", got, err)
	}
	other, err := c.SearchPullRequests(ctx, "author:@me", github.PageRequest{})
	if err != nil || len(other.Items) != 0 || other.HasMore() {
		t.Errorf("unseeded query should be empty: %+v, %v", other, err)
	}
}

func TestListIsOpenOnlyNewestFirst(t *testing.T) {
	c := fake.New()
	c.SetPullRequests(repo, []github.PullRequest{
		{Repo: repo, Number: 1, State: github.PRStateOpen, UpdatedAt: time.Unix(10, 0)},
		{Repo: repo, Number: 2, State: github.PRStateClosed, UpdatedAt: time.Unix(50, 0)},
		{Repo: repo, Number: 3, State: github.PRStateOpen, UpdatedAt: time.Unix(30, 0)},
		{Repo: repo, Number: 4, State: github.PRStateMerged, UpdatedAt: time.Unix(40, 0)},
	})
	p, err := c.ListPullRequests(context.Background(), repo, github.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, pr := range p.Items {
		got = append(got, pr.Number)
	}
	if diff := cmp.Diff([]int{3, 1}, got); diff != "" {
		t.Errorf("order mismatch:\n%s", diff)
	}
	if p.Total != 2 {
		t.Errorf("Total = %d, want 2", p.Total)
	}
}

func TestAddCommentFidelity(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown pull request", func(t *testing.T) {
		c := seeded()
		if _, err := c.AddComment(ctx, repo, 99, "x"); !errors.Is(err, github.ErrNotFound) {
			t.Errorf("want ErrNotFound, got %v", err)
		}
		if len(c.Comments()) != 0 {
			t.Error("failed comment must not be recorded")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		c := seeded()
		if _, err := c.AddComment(ctx, repo, 1, "  \n"); err == nil {
			t.Error("want error for empty body")
		}
		if _, err := c.ReplyToThread(ctx, "t1", ""); err == nil {
			t.Error("want error for empty reply")
		}
		l, _ := c.ListPullRequests(ctx, repo, github.PageRequest{})
		if l.Items[0].CommentCount != 0 {
			t.Error("rejected comment must not change counts")
		}
	})

	t.Run("comment counts follow", func(t *testing.T) {
		c := seeded()
		c.SetSearchResults("q", []github.PullRequest{{Repo: repo, Number: 1}})
		if _, err := c.AddComment(ctx, repo, 1, "hi"); err != nil {
			t.Fatal(err)
		}
		l, _ := c.ListPullRequests(ctx, repo, github.PageRequest{})
		d, _ := c.GetPullRequest(ctx, repo, 1)
		s, _ := c.SearchPullRequests(ctx, "q", github.PageRequest{})
		if l.Items[0].CommentCount != 1 || d.CommentCount != 1 || s.Items[0].CommentCount != 1 {
			t.Errorf("counts: list=%d detail=%d search=%d", l.Items[0].CommentCount, d.CommentCount, s.Items[0].CommentCount)
		}
	})

	t.Run("list-only pull request", func(t *testing.T) {
		c := seeded()
		if _, err := c.AddComment(ctx, repo, 3, "hi"); err != nil {
			t.Errorf("a PR known from the list can be commented on: %v", err)
		}
	})
}

// A view must be able to append to or edit what it got without touching the
// fake's state, and later fake writes must not change what the view holds.
func TestReturnedValuesAreNotAliased(t *testing.T) {
	ctx := context.Background()
	c := seeded()
	seed := github.PullRequestDetail{
		PullRequest: github.PullRequest{Repo: repo, Number: 1},
		Comments:    make([]github.IssueComment, 1, 8), // spare capacity invites aliasing
		Labels:      []github.Label{{Name: "a"}},
		Files:       []github.ChangedFile{{Path: "f"}},
	}
	seed.Comments[0] = github.IssueComment{ID: "c0", Reactions: []github.ReactionSummary{{Content: github.ReactionEyes, Count: 1}}}
	c.SetPullRequest(seed)
	// Mutating the seed after the fact must not leak in.
	seed.Comments[0].Body = "mutated seed"
	seed.Labels[0].Name = "mutated seed"

	got, err := c.GetPullRequest(ctx, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Comments[0].Body != "" || got.Labels[0].Name != "a" {
		t.Fatalf("seed leaked into the fake: %+v", got)
	}

	// Optimistic UI append plus an in-place edit...
	got.Comments = append(got.Comments, github.IssueComment{ID: "optimistic"})
	got.Comments[0].Reactions[0].Count = 99
	got.Labels[0].Name = "edited"
	// ...then the fake gains a comment.
	if _, err := c.AddComment(ctx, repo, 1, "real"); err != nil {
		t.Fatal(err)
	}
	if got.Comments[1].ID != "optimistic" {
		t.Errorf("fake overwrote the caller's slice: %+v", got.Comments)
	}

	again, err := c.GetPullRequest(ctx, repo, 1)
	if err != nil {
		t.Fatal(err)
	}
	if again.Comments[0].Reactions[0].Count != 1 || again.Labels[0].Name != "a" {
		t.Errorf("caller edits leaked into the fake: %+v", again)
	}
	if len(again.Comments) != 2 || again.Comments[1].Body != "real" {
		t.Errorf("unexpected comments: %+v", again.Comments)
	}
}

func TestThreadsAreNotAliased(t *testing.T) {
	ctx := context.Background()
	c := fake.New()
	threads := []github.ReviewThread{{ID: "t1", Comments: make([]github.ReviewComment, 1, 8)}}
	c.SetReviewThreads(repo, 1, threads)
	threads[0].Comments[0].Body = "mutated seed"

	p, err := c.ListReviewThreads(ctx, repo, 1, github.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Items[0].Comments[0].Body != "" {
		t.Fatal("seed leaked into the fake")
	}
	p.Items[0].Comments = append(p.Items[0].Comments, github.ReviewComment{ID: "optimistic"})
	mine := p.Items[0].Comments
	p.Items[0].Comments[0].Body = "edited"
	if _, err := c.ReplyToThread(ctx, "t1", "real"); err != nil {
		t.Fatal(err)
	}
	if mine[1].ID != "optimistic" {
		t.Error("fake overwrote the caller's slice")
	}
	again, _ := c.ListReviewThreads(ctx, repo, 1, github.PageRequest{})
	if got := again.Items[0].Comments; len(got) != 2 || got[0].Body != "" || got[1].Body != "real" {
		t.Errorf("unexpected comments: %+v", got)
	}
}

// Run with -race (make test does): views call the client from tea.Cmd goroutines.
func TestConcurrentUse(t *testing.T) {
	c := seeded()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				switch i % 4 {
				case 0:
					_, _ = c.AddComment(ctx, repo, 1, "x")
				case 1:
					_, _ = c.GetPullRequest(ctx, repo, 1)
				case 2:
					_, _ = c.ReplyToThread(ctx, "t1", "x")
				default:
					_, _ = c.ListReviewThreads(ctx, repo, 1, github.PageRequest{})
					_, _ = c.ListPullRequests(ctx, repo, github.PageRequest{})
				}
				_ = c.Calls()
			}
		}()
	}
	wg.Wait()
	if got := len(c.Comments()); got != 40 {
		t.Errorf("recorded %d comments, want 40", got)
	}
}
