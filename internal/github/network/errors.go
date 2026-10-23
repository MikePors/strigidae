package network

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	githubnetwork "github.com/cli/go-gh/v2/pkg/api"

	"github.com/MikePors/strigidae/internal/github"
)

func parseReset(value string) (time.Time, bool) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// mapError converts go-gh errors into the typed errors of package github,
// keeping the original error in the chain.
func (client *Client) mapError(err error) error {
	var rateLimitError *github.RateLimitError
	if errors.As(err, &rateLimitError) {
		return err
	}
	var httpErr *githubnetwork.HTTPError
	if errors.As(err, &httpErr) {
		return client.mapHTTPError(httpErr)
	}
	var graphqlErr *githubnetwork.GraphQLError
	if errors.As(err, &graphqlErr) {
		return client.mapGraphQLError(graphqlErr)
	}
	return err
}

func (client *Client) mapHTTPError(httpError *githubnetwork.HTTPError) error {
	switch {
	case httpError.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", github.ErrNotAuthenticated, httpError)
	case httpError.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %w", github.ErrNotFound, httpError)
	case isRateLimited(httpError):
		return &github.RateLimitError{ResetAt: client.httpResetAt(httpError), Err: httpError}
	case httpError.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", github.ErrForbidden, httpError)
	}
	return httpError
}

func isRateLimited(httpError *githubnetwork.HTTPError) bool {
	if httpError.StatusCode != http.StatusForbidden && httpError.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if httpError.StatusCode == http.StatusTooManyRequests || httpError.Headers.Get("Retry-After") != "" ||
		httpError.Headers.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return strings.Contains(strings.ToLower(httpError.Message), "rate limit")
}

// defaultSecondaryWait is the documented advice when a secondary limit gives
// no Retry-After: wait at least a minute.
const defaultSecondaryWait = time.Minute

// httpResetAt works out when a rate-limited REST request may be retried.
// X-RateLimit-Reset belongs to the primary bucket, so it is only used when
// that bucket is actually exhausted.
func (client *Client) httpResetAt(httpError *githubnetwork.HTTPError) time.Time {
	if value := httpError.Headers.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return client.now().Add(time.Duration(seconds) * time.Second)
		}
		if timestamp, err := http.ParseTime(value); err == nil {
			return timestamp
		}
	}
	if httpError.Headers.Get("X-RateLimit-Remaining") == "0" {
		if reset, ok := parseReset(httpError.Headers.Get("X-RateLimit-Reset")); ok {
			return reset
		}
	}
	return client.now().Add(defaultSecondaryWait)
}

// isScopeError reports whether err is a GraphQL permission/scope failure.
func isScopeError(err error) bool {
	var graphqlErr *githubnetwork.GraphQLError
	if err == nil || !errors.As(err, &graphqlErr) {
		return false
	}
	for _, item := range graphqlErr.Errors {
		if item.Type == "FORBIDDEN" || item.Type == "INSUFFICIENT_SCOPES" {
			return true
		}
	}
	return false
}

func (client *Client) mapGraphQLError(graphqlError *githubnetwork.GraphQLError) error {
	for _, item := range graphqlError.Errors {
		switch item.Type {
		case "RATE_LIMITED":
			return &github.RateLimitError{ResetAt: client.limit.resetFor("graphql"), Err: graphqlError}
		case "NOT_FOUND":
			return fmt.Errorf("%w: %w", github.ErrNotFound, graphqlError)
		case "FORBIDDEN", "INSUFFICIENT_SCOPES":
			return fmt.Errorf("%w: %w", github.ErrForbidden, graphqlError)
		}
	}
	return graphqlError
}
