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
	// SetFileViewed marks the file at path as viewed (or, with viewed false,
	// clears the mark) for the viewer. pullRequestID is PullRequest.ID.
	SetFileViewed(ctx context.Context, pullRequestID, path string, viewed bool) error
	// GetMergeStatus returns what a merge needs to know right now: the pull
	// request's node ID, head commit and base branch, and the repository's
	// allowed merge methods and branch deletion setting.
	GetMergeStatus(ctx context.Context, repo Repo, number int) (MergeStatus, error)
	// MergePullRequest merges the pull request with in.Method and GitHub's
	// default commit title and message. GitHub's refusal (failing checks,
	// conflicts, a moved head) comes back as an error.
	MergePullRequest(ctx context.Context, in MergeInput) error
	// StartReview creates the viewer's pending review on in.CommitOID and
	// returns it. GitHub allows one pending review per user per pull request,
	// so this fails when one exists; call GetReviewStatus first.
	StartReview(ctx context.Context, in StartReviewInput) (PendingReview, error)
	// AddReviewThread adds an inline comment, as a new thread, to the pending
	// review in.PendingReviewID and returns the thread. The comment stays
	// private until the review is submitted. A pending review that no longer
	// exists yields ErrNotFound.
	AddReviewThread(ctx context.Context, in AddThreadInput) (ReviewThread, error)
	// GetReviewComment returns one review comment by node ID, with its current
	// state (Pending) and what the viewer may do with it. A comment that does
	// not exist any more yields ErrNotFound.
	GetReviewComment(ctx context.Context, commentID string) (ReviewComment, error)
	// UpdateReviewComment changes the body of a review comment, pending or
	// published, and returns it. GitHub decides by permissions (see
	// ReviewComment.ViewerCanUpdate): the viewer's own comments, and with write
	// access other people's too.
	UpdateReviewComment(ctx context.Context, commentID, body string) (ReviewComment, error)
	// DeleteReviewComment deletes a review comment, pending or published, under
	// the same permissions (see ReviewComment.ViewerCanDelete).
	// Deleting a thread's only comment removes the thread.
	DeleteReviewComment(ctx context.Context, commentID string) error
}
