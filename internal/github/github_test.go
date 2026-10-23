package github

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestParseRepository(test *testing.T) {
	tests := []struct {
		in      string
		want    Repository
		wantErr bool
	}{
		{in: "MikePors/strigidae", want: Repository{"MikePors", "strigidae"}},
		{in: " octo/widgets ", want: Repository{"octo", "widgets"}},
		{in: "a/b.c-d_e", want: Repository{"a", "b.c-d_e"}},
		{in: "", wantErr: true},
		{in: "noslash", wantErr: true},
		{in: "/name", wantErr: true},
		{in: "owner/", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "a b/c", wantErr: true},
	}
	for _, testCase := range tests {
		got, err := ParseRepository(testCase.in)
		if (err != nil) != testCase.wantErr {
			test.Errorf("ParseRepo(%q) error = %v, wantErr %v", testCase.in, err, testCase.wantErr)
			continue
		}
		if got != testCase.want {
			test.Errorf("ParseRepo(%q) = %v, want %v", testCase.in, got, testCase.want)
		}
		if err == nil && got.String() != testCase.want.Owner+"/"+testCase.want.Name {
			test.Errorf("String() = %q", got.String())
		}
	}
}

func TestPageRequestSize(test *testing.T) {
	tests := []struct{ limit, want int }{
		{0, DefaultPageSize}, {-1, DefaultPageSize}, {7, 7}, {100, 100}, {500, MaximumPageSize},
	}
	for _, testCase := range tests {
		if got := (PageRequest{Limit: testCase.limit}).Size(); got != testCase.want {
			test.Errorf("Size(%d) = %d, want %d", testCase.limit, got, testCase.want)
		}
	}
}

func TestRateLimitError(test *testing.T) {
	now := time.Unix(1000, 0)
	rateLimitError := &RateLimitError{ResetAt: now.Add(90 * time.Second), Err: errors.New("api")}
	if got := rateLimitError.RetryAfter(now); got != 90*time.Second {
		test.Errorf("RetryAfter = %v", got)
	}
	if got := rateLimitError.RetryAfter(now.Add(time.Hour)); got != 0 {
		test.Errorf("RetryAfter after reset = %v", got)
	}
	if got := (&RateLimitError{}).RetryAfter(now); got != 0 {
		test.Errorf("RetryAfter unknown = %v", got)
	}
	// The message must not depend on the local time zone.
	zoned := &RateLimitError{ResetAt: time.Date(2026, 10, 19, 10, 0, 0, 0, time.FixedZone("X", -7*3600))}
	if got, want := zoned.Error(), "GitHub API rate limit exceeded; resets at 2026-10-19T17:00:00Z"; got != want {
		test.Errorf("Error() = %q, want %q", got, want)
	}
	if got := (&RateLimitError{}).Error(); got != "GitHub API rate limit exceeded" {
		test.Errorf("Error() = %q", got)
	}
	wrapped := fmt.Errorf("list: %w", rateLimitError)
	var target *RateLimitError
	if !errors.As(wrapped, &target) || target != rateLimitError {
		test.Error("errors.As should find RateLimitError")
	}
}

func TestNotAuthenticatedMessage(test *testing.T) {
	if got := ErrNotAuthenticated.Error(); got == "" || !errors.Is(fmt.Errorf("x: %w", ErrNotAuthenticated), ErrNotAuthenticated) {
		test.Errorf("unexpected: %q", got)
	}
}

func TestReactionEmoji(test *testing.T) {
	if ReactionThumbsUp.Emoji() != "👍" || ReactionContent("NEW").Emoji() != "NEW" {
		test.Error("unexpected emoji mapping")
	}
}
