package network

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/MikePors/strigidae/internal/github"
)

func parseReset(v string) (time.Time, bool) {
	secs, err := strconv.ParseInt(v, 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}, false
	}
	return time.Unix(secs, 0), true
}

// mapError converts go-gh errors into the typed errors of package github,
// keeping the original error in the chain.
func (c *Client) mapError(err error) error {
	var rl *github.RateLimitError
	if errors.As(err, &rl) {
		return err
	}
	var httpErr *ghapi.HTTPError
	if errors.As(err, &httpErr) {
		return c.mapHTTPError(httpErr)
	}
	var gqlErr *ghapi.GraphQLError
	if errors.As(err, &gqlErr) {
		return c.mapGraphQLError(gqlErr)
	}
	return err
}

func (c *Client) mapHTTPError(e *ghapi.HTTPError) error {
	switch {
	case e.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", github.ErrNotAuthenticated, e)
	case e.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %w", github.ErrNotFound, e)
	case isRateLimited(e):
		return &github.RateLimitError{ResetAt: c.httpResetAt(e), Err: e}
	case e.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", github.ErrForbidden, e)
	}
	return e
}

func isRateLimited(e *ghapi.HTTPError) bool {
	if e.StatusCode != http.StatusForbidden && e.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if e.StatusCode == http.StatusTooManyRequests || e.Headers.Get("Retry-After") != "" ||
		e.Headers.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(e.Message), "rate limit")
}

// defaultSecondaryWait is the documented advice when a secondary limit gives
// no Retry-After: wait at least a minute.
const defaultSecondaryWait = time.Minute

// httpResetAt works out when a rate-limited REST request may be retried.
// X-RateLimit-Reset belongs to the primary bucket, so it is only used when
// that bucket is actually exhausted.
func (c *Client) httpResetAt(e *ghapi.HTTPError) time.Time {
	if v := e.Headers.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return c.now().Add(time.Duration(secs) * time.Second)
		}
		if t, err := http.ParseTime(v); err == nil {
			return t
		}
	}
	if e.Headers.Get("X-RateLimit-Remaining") == "0" {
		if reset, ok := parseReset(e.Headers.Get("X-RateLimit-Reset")); ok {
			return reset
		}
	}
	return c.now().Add(defaultSecondaryWait)
}

// isScopeError reports whether err is a GraphQL permission/scope failure.
func isScopeError(err error) bool {
	var gqlErr *ghapi.GraphQLError
	if err == nil || !errors.As(err, &gqlErr) {
		return false
	}
	for _, item := range gqlErr.Errors {
		if item.Type == "FORBIDDEN" || item.Type == "INSUFFICIENT_SCOPES" {
			return true
		}
	}
	return false
}

func (c *Client) mapGraphQLError(e *ghapi.GraphQLError) error {
	for _, item := range e.Errors {
		switch item.Type {
		case "RATE_LIMITED":
			return &github.RateLimitError{ResetAt: c.limit.resetFor("graphql"), Err: e}
		case "NOT_FOUND":
			return fmt.Errorf("%w: %w", github.ErrNotFound, e)
		case "FORBIDDEN", "INSUFFICIENT_SCOPES":
			return fmt.Errorf("%w: %w", github.ErrForbidden, e)
		}
	}
	return e
}
