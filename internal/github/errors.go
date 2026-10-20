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
)

// RateLimitError means GitHub refused the request because a rate limit was hit.
type RateLimitError struct {
	// ResetAt is when requests may resume; zero if GitHub did not say.
	ResetAt time.Time
	// Err is the underlying API error.
	Err error
}

// Error implements error.
func (e *RateLimitError) Error() string {
	if e.ResetAt.IsZero() {
		return "GitHub API rate limit exceeded"
	}
	return fmt.Sprintf("GitHub API rate limit exceeded; resets at %s", e.ResetAt.Local().Format(time.TimeOnly))
}

// Unwrap returns the underlying API error.
func (e *RateLimitError) Unwrap() error { return e.Err }

// RetryAfter returns how long until the limit resets relative to now, never
// negative. It is zero when the reset time is unknown.
func (e *RateLimitError) RetryAfter(now time.Time) time.Duration {
	if e.ResetAt.IsZero() || !e.ResetAt.After(now) {
		return 0
	}
	return e.ResetAt.Sub(now)
}
