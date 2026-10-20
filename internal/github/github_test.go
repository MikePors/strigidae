package github

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		in      string
		want    Repo
		wantErr bool
	}{
		{in: "MikePors/strigidae", want: Repo{"MikePors", "strigidae"}},
		{in: " octo/widgets ", want: Repo{"octo", "widgets"}},
		{in: "a/b.c-d_e", want: Repo{"a", "b.c-d_e"}},
		{in: "", wantErr: true},
		{in: "noslash", wantErr: true},
		{in: "/name", wantErr: true},
		{in: "owner/", wantErr: true},
		{in: "a/b/c", wantErr: true},
		{in: "a b/c", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseRepo(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseRepo(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseRepo(%q) = %v, want %v", tt.in, got, tt.want)
		}
		if err == nil && got.String() != tt.want.Owner+"/"+tt.want.Name {
			t.Errorf("String() = %q", got.String())
		}
	}
}

func TestPageRequestSize(t *testing.T) {
	tests := []struct{ limit, want int }{
		{0, DefaultPageSize}, {-1, DefaultPageSize}, {7, 7}, {100, 100}, {500, MaxPageSize},
	}
	for _, tt := range tests {
		if got := (PageRequest{Limit: tt.limit}).Size(); got != tt.want {
			t.Errorf("Size(%d) = %d, want %d", tt.limit, got, tt.want)
		}
	}
}

func TestRateLimitError(t *testing.T) {
	now := time.Unix(1000, 0)
	e := &RateLimitError{ResetAt: now.Add(90 * time.Second), Err: errors.New("api")}
	if got := e.RetryAfter(now); got != 90*time.Second {
		t.Errorf("RetryAfter = %v", got)
	}
	if got := e.RetryAfter(now.Add(time.Hour)); got != 0 {
		t.Errorf("RetryAfter after reset = %v", got)
	}
	if got := (&RateLimitError{}).RetryAfter(now); got != 0 {
		t.Errorf("RetryAfter unknown = %v", got)
	}
	// The message must not depend on the local time zone.
	zoned := &RateLimitError{ResetAt: time.Date(2026, 10, 19, 10, 0, 0, 0, time.FixedZone("X", -7*3600))}
	if got, want := zoned.Error(), "GitHub API rate limit exceeded; resets at 2026-10-19T17:00:00Z"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := (&RateLimitError{}).Error(); got != "GitHub API rate limit exceeded" {
		t.Errorf("Error() = %q", got)
	}
	wrapped := fmt.Errorf("list: %w", e)
	var target *RateLimitError
	if !errors.As(wrapped, &target) || target != e {
		t.Error("errors.As should find RateLimitError")
	}
}

func TestNotAuthenticatedMessage(t *testing.T) {
	if got := ErrNotAuthenticated.Error(); got == "" || !errors.Is(fmt.Errorf("x: %w", ErrNotAuthenticated), ErrNotAuthenticated) {
		t.Errorf("unexpected: %q", got)
	}
}

func TestReactionEmoji(t *testing.T) {
	if ReactionThumbsUp.Emoji() != "👍" || ReactionContent("NEW").Emoji() != "NEW" {
		t.Error("unexpected emoji mapping")
	}
}
