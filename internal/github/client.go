package github

import "context"

// Client is everything the UI needs from GitHub. All methods are safe for
// concurrent use. Errors wrap ErrNotAuthenticated, ErrNotFound, ErrForbidden
// or *RateLimitError where applicable; test with errors.Is / errors.As.
type Client interface {
	// ListPullRequests lists open pull requests in repo, most recently updated first.
	ListPullRequests(ctx context.Context, repo Repo, page PageRequest) (Page[PullRequest], error)
	// SearchPullRequests runs a GitHub search query restricted to pull
	// requests ("is:pr" is added automatically), e.g. "review-requested:@me is:open".
	SearchPullRequests(ctx context.Context, query string, page PageRequest) (Page[PullRequest], error)
	// GetPullRequest returns one pull request with all its files, conversation
	// comments and review summaries.
	GetPullRequest(ctx context.Context, repo Repo, number int) (PullRequestDetail, error)
	// GetDiff returns the raw unified diff of the pull request.
	GetDiff(ctx context.Context, repo Repo, number int) (string, error)
	// ListReviewThreads lists review threads, including resolved and outdated ones.
	ListReviewThreads(ctx context.Context, repo Repo, number int, page PageRequest) (Page[ReviewThread], error)
	// ReplyToThread posts a reply to the thread with the given ID and returns the new comment.
	ReplyToThread(ctx context.Context, threadID, body string) (ReviewComment, error)
	// AddComment posts a top-level comment on the pull request.
	AddComment(ctx context.Context, repo Repo, number int, body string) (IssueComment, error)
	// GetReviewStatus returns what a review submission needs to know right
	// now: the pull request's node ID, its current head commit, and the
	// viewer's pending review if there is one. It is a small query, cheap
	// enough to run just before asking for confirmation.
	GetReviewStatus(ctx context.Context, repo Repo, number int) (ReviewStatus, error)
	// SubmitReview submits a review verdict. With in.PendingReviewID set it
	// submits that pending review (so its inline comments go too); otherwise
	// it creates and submits a new review on in.CommitOID. A pending review
	// that no longer exists yields ErrNotFound.
	SubmitReview(ctx context.Context, in SubmitReviewInput) error
}
