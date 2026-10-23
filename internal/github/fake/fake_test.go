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

var repository = github.Repository{Owner: "o", Name: "r"}

func seeded() *fake.Client {
	client := fake.New()
	client.Now = func() time.Time { return time.Unix(100, 0) }
	var pullRequests []github.PullRequest
	for i := 1; i <= 5; i++ {
		pullRequests = append(pullRequests, github.PullRequest{Repository: repository, Number: i, State: github.PullRequestStateOpen, UpdatedAt: time.Unix(int64(1000-i), 0)})
	}
	client.SetPullRequests(repository, pullRequests)
	client.SetPullRequest(github.PullRequestDetail{PullRequest: github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1}})
	client.SetDiff(repository, 1, "diff")
	client.SetReviewThreads(repository, 1, []github.ReviewThread{{Identifier: "t1"}, {Identifier: "t2"}})
	return client
}

func TestPagination(test *testing.T) {
	client := seeded()
	ctx := context.Background()
	var numbers []int
	var request github.PageRequest
	request.Limit = 2
	pages := 0
	for {
		page, err := client.ListPullRequests(ctx, repository, request)
		if err != nil {
			test.Fatal(err)
		}
		pages++
		for _, pullRequest := range page.Items {
			numbers = append(numbers, pullRequest.Number)
		}
		if !page.HasMore() {
			break
		}
		request.After = page.Next
	}
	if diff := cmp.Diff([]int{1, 2, 3, 4, 5}, numbers); diff != "" {
		test.Errorf("numbers mismatch:\n%s", diff)
	}
	if pages != 3 {
		test.Errorf("pages = %d, want 3", pages)
	}
	if _, err := client.ListPullRequests(ctx, repository, github.PageRequest{After: "bogus"}); err == nil {
		test.Error("want error for bad cursor")
	}
}

func TestReplyAndCommentAreRecorded(test *testing.T) {
	client := seeded()
	ctx := context.Background()

	reviewComment, err := client.ReplyToThread(ctx, "t2", "hello")
	if err != nil {
		test.Fatal(err)
	}
	if reviewComment.Author != "me" || reviewComment.Body != "hello" || !reviewComment.CreatedAt.Equal(time.Unix(100, 0)) {
		test.Errorf("unexpected reply: %+v", reviewComment)
	}
	if diff := cmp.Diff([]fake.Reply{{ThreadIdentifier: "t2", Body: "hello"}}, client.Replies()); diff != "" {
		test.Errorf("replies mismatch:\n%s", diff)
	}
	threads, err := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if err != nil {
		test.Fatal(err)
	}
	if got := len(threads.Items[1].Comments); got != 1 {
		test.Errorf("reply not visible in thread: %d comments", got)
	}

	if _, err := client.AddComment(ctx, repository, 1, "top"); err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]fake.Comment{{Repository: repository, Number: 1, Body: "top"}}, client.Comments()); diff != "" {
		test.Errorf("comments mismatch:\n%s", diff)
	}
	detail, err := client.GetPullRequest(ctx, repository, 1)
	if err != nil {
		test.Fatal(err)
	}
	if len(detail.Comments) != 1 || detail.CommentCount != 1 {
		test.Errorf("comment not visible in detail: %+v", detail)
	}
}

func TestErrors(test *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")

	test.Run("per method", func(test *testing.T) {
		client := seeded()
		client.SetError(fake.MethodGetDiff, boom)
		if _, err := client.GetDiff(ctx, repository, 1); !errors.Is(err, boom) {
			test.Errorf("want boom, got %v", err)
		}
		if _, err := client.ListPullRequests(ctx, repository, github.PageRequest{}); err != nil {
			test.Errorf("other methods should work: %v", err)
		}
		client.SetError(fake.MethodGetDiff, nil)
		if _, err := client.GetDiff(ctx, repository, 1); err != nil {
			test.Errorf("error should be cleared: %v", err)
		}
	})

	test.Run("all methods", func(test *testing.T) {
		client := seeded()
		client.SetError("", &github.RateLimitError{})
		_, err := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
		var rateLimitError *github.RateLimitError
		if !errors.As(err, &rateLimitError) {
			test.Errorf("want RateLimitError, got %v", err)
		}
		if _, err := client.ReplyToThread(ctx, "t1", "x"); err == nil {
			test.Error("want error")
		}
		if len(client.Replies()) != 0 {
			test.Error("failed reply must not be recorded")
		}
	})

	test.Run("not found", func(test *testing.T) {
		client := seeded()
		if _, err := client.GetPullRequest(ctx, repository, 99); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("want ErrNotFound, got %v", err)
		}
		if _, err := client.ReplyToThread(ctx, "nope", "x"); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("want ErrNotFound, got %v", err)
		}
	})

	test.Run("cancelled context", func(test *testing.T) {
		client := seeded()
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := client.GetDiff(cctx, repository, 1); !errors.Is(err, context.Canceled) {
			test.Errorf("want context.Canceled, got %v", err)
		}
	})
}

func TestCalls(test *testing.T) {
	client := seeded()
	ctx := context.Background()
	_, _ = client.GetDiff(ctx, repository, 1)
	_, _ = client.SearchPullRequests(ctx, "q", github.PageRequest{After: "3", Limit: 5})
	want := []fake.Call{
		{Method: fake.MethodGetDiff, Repository: repository, Number: 1},
		{Method: fake.MethodSearchPullRequests, Query: "q", Page: github.PageRequest{After: "3", Limit: 5}},
	}
	if diff := cmp.Diff(want, client.Calls()); diff != "" {
		test.Errorf("calls mismatch:\n%s", diff)
	}
}

func TestSearchIsKeyedByQuery(test *testing.T) {
	client := seeded()
	ctx := context.Background()
	client.SetSearchResults("review-requested:@me", []github.PullRequest{{Repository: repository, Number: 7}})
	got, err := client.SearchPullRequests(ctx, "review-requested:@me", github.PageRequest{})
	if err != nil || len(got.Items) != 1 || got.Items[0].Number != 7 {
		test.Fatalf("seeded query: %+v, %v", got, err)
	}
	other, err := client.SearchPullRequests(ctx, "author:@me", github.PageRequest{})
	if err != nil || len(other.Items) != 0 || other.HasMore() {
		test.Errorf("unseeded query should be empty: %+v, %v", other, err)
	}
}

func TestListIsOpenOnlyNewestFirst(test *testing.T) {
	client := fake.New()
	client.SetPullRequests(repository, []github.PullRequest{
		{Repository: repository, Number: 1, State: github.PullRequestStateOpen, UpdatedAt: time.Unix(10, 0)},
		{Repository: repository, Number: 2, State: github.PullRequestStateClosed, UpdatedAt: time.Unix(50, 0)},
		{Repository: repository, Number: 3, State: github.PullRequestStateOpen, UpdatedAt: time.Unix(30, 0)},
		{Repository: repository, Number: 4, State: github.PullRequestStateMerged, UpdatedAt: time.Unix(40, 0)},
	})
	page, err := client.ListPullRequests(context.Background(), repository, github.PageRequest{})
	if err != nil {
		test.Fatal(err)
	}
	var got []int
	for _, pullRequest := range page.Items {
		got = append(got, pullRequest.Number)
	}
	if diff := cmp.Diff([]int{3, 1}, got); diff != "" {
		test.Errorf("order mismatch:\n%s", diff)
	}
	if page.Total != 2 {
		test.Errorf("Total = %d, want 2", page.Total)
	}
}

func TestAddCommentFidelity(test *testing.T) {
	ctx := context.Background()

	test.Run("unknown pull request", func(test *testing.T) {
		client := seeded()
		if _, err := client.AddComment(ctx, repository, 99, "x"); !errors.Is(err, github.ErrNotFound) {
			test.Errorf("want ErrNotFound, got %v", err)
		}
		if len(client.Comments()) != 0 {
			test.Error("failed comment must not be recorded")
		}
	})

	test.Run("empty body", func(test *testing.T) {
		client := seeded()
		if _, err := client.AddComment(ctx, repository, 1, "  \n"); err == nil {
			test.Error("want error for empty body")
		}
		if _, err := client.ReplyToThread(ctx, "t1", ""); err == nil {
			test.Error("want error for empty reply")
		}
		page, _ := client.ListPullRequests(ctx, repository, github.PageRequest{})
		if page.Items[0].CommentCount != 0 {
			test.Error("rejected comment must not change counts")
		}
	})

	test.Run("comment counts follow", func(test *testing.T) {
		client := seeded()
		client.SetSearchResults("q", []github.PullRequest{{Repository: repository, Number: 1}})
		if _, err := client.AddComment(ctx, repository, 1, "hi"); err != nil {
			test.Fatal(err)
		}
		listPage, _ := client.ListPullRequests(ctx, repository, github.PageRequest{})
		detail, _ := client.GetPullRequest(ctx, repository, 1)
		searchPage, _ := client.SearchPullRequests(ctx, "q", github.PageRequest{})
		if listPage.Items[0].CommentCount != 1 || detail.CommentCount != 1 || searchPage.Items[0].CommentCount != 1 {
			test.Errorf("counts: list=%d detail=%d search=%d", listPage.Items[0].CommentCount, detail.CommentCount, searchPage.Items[0].CommentCount)
		}
	})

	test.Run("list-only pull request", func(test *testing.T) {
		client := seeded()
		if _, err := client.AddComment(ctx, repository, 3, "hi"); err != nil {
			test.Errorf("a PR known from the list can be commented on: %v", err)
		}
	})
}

// A view must be able to append to or edit what it got without touching the
// fake's state, and later fake writes must not change what the view holds.
func TestReturnedValuesAreNotAliased(test *testing.T) {
	ctx := context.Background()
	client := seeded()
	seed := github.PullRequestDetail{
		PullRequest: github.PullRequest{Repository: repository, Number: 1},
		Comments:    make([]github.IssueComment, 1, 8), // spare capacity invites aliasing
		Labels:      []github.Label{{Name: "a"}},
		Files:       []github.ChangedFile{{Path: "f"}},
	}
	seed.Comments[0] = github.IssueComment{Identifier: "c0", Reactions: []github.ReactionSummary{{Content: github.ReactionEyes, Count: 1}}}
	client.SetPullRequest(seed)
	// Mutating the seed after the fact must not leak in.
	seed.Comments[0].Body = "mutated seed"
	seed.Labels[0].Name = "mutated seed"

	got, err := client.GetPullRequest(ctx, repository, 1)
	if err != nil {
		test.Fatal(err)
	}
	if got.Comments[0].Body != "" || got.Labels[0].Name != "a" {
		test.Fatalf("seed leaked into the fake: %+v", got)
	}

	// Optimistic UI append plus an in-place edit...
	got.Comments = append(got.Comments, github.IssueComment{Identifier: "optimistic"})
	got.Comments[0].Reactions[0].Count = 99
	got.Labels[0].Name = "edited"
	// ...then the fake gains a comment.
	if _, err := client.AddComment(ctx, repository, 1, "real"); err != nil {
		test.Fatal(err)
	}
	if got.Comments[1].Identifier != "optimistic" {
		test.Errorf("fake overwrote the caller's slice: %+v", got.Comments)
	}

	again, err := client.GetPullRequest(ctx, repository, 1)
	if err != nil {
		test.Fatal(err)
	}
	if again.Comments[0].Reactions[0].Count != 1 || again.Labels[0].Name != "a" {
		test.Errorf("caller edits leaked into the fake: %+v", again)
	}
	if len(again.Comments) != 2 || again.Comments[1].Body != "real" {
		test.Errorf("unexpected comments: %+v", again.Comments)
	}
}

func TestThreadsAreNotAliased(test *testing.T) {
	ctx := context.Background()
	client := fake.New()
	threads := []github.ReviewThread{{Identifier: "t1", Comments: make([]github.ReviewComment, 1, 8)}}
	client.SetReviewThreads(repository, 1, threads)
	threads[0].Comments[0].Body = "mutated seed"

	page, err := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if err != nil {
		test.Fatal(err)
	}
	if page.Items[0].Comments[0].Body != "" {
		test.Fatal("seed leaked into the fake")
	}
	page.Items[0].Comments = append(page.Items[0].Comments, github.ReviewComment{Identifier: "optimistic"})
	mine := page.Items[0].Comments
	page.Items[0].Comments[0].Body = "edited"
	if _, err := client.ReplyToThread(ctx, "t1", "real"); err != nil {
		test.Fatal(err)
	}
	if mine[1].Identifier != "optimistic" {
		test.Error("fake overwrote the caller's slice")
	}
	again, _ := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if got := again.Items[0].Comments; len(got) != 2 || got[0].Body != "" || got[1].Body != "real" {
		test.Errorf("unexpected comments: %+v", got)
	}
}

// Run with -race (make test does): views call the client from tea.Cmd goroutines.
func TestConcurrentUse(test *testing.T) {
	client := seeded()
	ctx := context.Background()
	var waitGroup sync.WaitGroup
	for i := range 8 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for range 20 {
				switch i % 4 {
				case 0:
					_, _ = client.AddComment(ctx, repository, 1, "x")
				case 1:
					_, _ = client.GetPullRequest(ctx, repository, 1)
				case 2:
					_, _ = client.ReplyToThread(ctx, "t1", "x")
				default:
					_, _ = client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
					_, _ = client.ListPullRequests(ctx, repository, github.PageRequest{})
				}
				_ = client.Calls()
			}
		}()
	}
	waitGroup.Wait()
	if got := len(client.Comments()); got != 40 {
		test.Errorf("recorded %d comments, want 40", got)
	}
}

func TestSetFileViewed(test *testing.T) {
	client := fake.New()
	client.SetPullRequest(github.PullRequestDetail{
		PullRequest: github.PullRequest{Identifier: "PR_1", Repository: repository, Number: 1},
		Files:       []github.ChangedFile{{Path: "a.go", Viewed: github.FileDismissed}, {Path: "b.go"}},
	})
	ctx := context.Background()
	if err := client.SetFileViewed(ctx, "PR_1", "a.go", true); err != nil {
		test.Fatal(err)
	}
	detail, _ := client.GetPullRequest(ctx, repository, 1)
	if detail.Files[0].Viewed != github.FileViewed || detail.Files[1].Viewed != "" {
		test.Errorf("files = %+v", detail.Files)
	}
	if err := client.SetFileViewed(ctx, "PR_1", "a.go", false); err != nil {
		test.Fatal(err)
	}
	detail, _ = client.GetPullRequest(ctx, repository, 1)
	if detail.Files[0].Viewed != github.FileUnviewed {
		test.Errorf("files = %+v", detail.Files)
	}
	if err := client.SetFileViewed(ctx, "PR_1", "missing.go", true); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("unknown path: err = %v", err)
	}
	if err := client.SetFileViewed(ctx, "PR_9", "a.go", true); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("unknown PR: err = %v", err)
	}
	boom := errors.New("boom")
	client.SetError(fake.MethodSetFileViewed, boom)
	if err := client.SetFileViewed(ctx, "PR_1", "a.go", true); !errors.Is(err, boom) {
		test.Errorf("injected: err = %v", err)
	}
	calls := client.Calls()
	last := calls[len(calls)-1]
	if last.Method != fake.MethodSetFileViewed || last.PullRequestIdentifier != "PR_1" || last.Path != "a.go" || !last.Viewed {
		test.Errorf("last call = %+v", last)
	}
}
