package api

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

func (c *Client) httpResetAt(e *ghapi.HTTPError) time.Time {
	if secs, err := strconv.Atoi(e.Headers.Get("Retry-After")); err == nil {
		return c.now().Add(time.Duration(secs) * time.Second)
	}
	reset, _ := parseReset(e.Headers.Get("X-RateLimit-Reset"))
	return reset
}

func (c *Client) mapGraphQLError(e *ghapi.GraphQLError) error {
	for _, item := range e.Errors {
		switch item.Type {
		case "RATE_LIMITED":
			return &github.RateLimitError{ResetAt: c.limit.exhaustedReset(), Err: e}
		case "NOT_FOUND":
			return fmt.Errorf("%w: %w", github.ErrNotFound, e)
		case "FORBIDDEN", "INSUFFICIENT_SCOPES":
			return fmt.Errorf("%w: %w", github.ErrForbidden, e)
		}
	}
	return e
}
