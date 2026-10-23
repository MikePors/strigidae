package github

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotAuthenticated means no usable token was found, or GitHub rejected it.
	ErrNotAuthenticated = errors.New("not authenticated with GitHub: run `gh auth login` (or set GH_TOKEN)")
	// ErrNotFound means the repository, pull request or thread does not exist
	// or is not visible to the authenticated user.
	ErrNotFound = errors.New("not found")
	// ErrForbidden means the token lacks permission for the operation.
	ErrForbidden = errors.New("forbidden")
	// ErrCommentRejected means GitHub did not create a review comment, as it
	// answers for a position that is not part of the diff.
	ErrCommentRejected = errors.New("comment not created")
	// ErrOwnPullRequest means GitHub refused an approval or a request for
	// changes because the viewer authored the pull request.
	ErrOwnPullRequest = errors.New("cannot approve or request changes on your own pull request")
	// ErrDiffTooLarge means GitHub refuses to produce the diff over the API
	// because the pull request exceeds its file or line limits.
	ErrDiffTooLarge = errors.New("diff too large for the GitHub API")
)

// RateLimitError means GitHub refused the request because a rate limit was hit.
type RateLimitError struct {
	// ResetAt is when requests may resume; zero if GitHub did not say.
	ResetAt time.Time
	// Err is the underlying API error.
	Err error
}

// Error implements error.
func (rateLimitError *RateLimitError) Error() string {
	if rateLimitError.ResetAt.IsZero() {
		return "GitHub API rate limit exceeded"
	}
	return fmt.Sprintf("GitHub API rate limit exceeded; resets at %s", rateLimitError.ResetAt.UTC().Format(time.RFC3339))
}

// Unwrap returns the underlying API error.
func (rateLimitError *RateLimitError) Unwrap() error { return rateLimitError.Err }

// RetryAfter returns how long until the limit resets relative to now, never
// negative. It is zero when the reset time is unknown.
func (rateLimitError *RateLimitError) RetryAfter(now time.Time) time.Duration {
	if rateLimitError.ResetAt.IsZero() || !rateLimitError.ResetAt.After(now) {
		return 0
	}
	return rateLimitError.ResetAt.Sub(now)
}
