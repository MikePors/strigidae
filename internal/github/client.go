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
	// SetFileViewed marks the file at path as viewed (or, with viewed false,
	// clears the mark) for the viewer. pullRequestID is PullRequest.ID.
	SetFileViewed(ctx context.Context, pullRequestID, path string, viewed bool) error
}
