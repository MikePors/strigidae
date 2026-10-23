package network

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestHasPRQualifier(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"review-requested:@me", false},
		{"is:pr review-requested:@me", true},
		{"review-requested:@me is:pr", true},
		{"type:pr author:octocat", true},
		{"is:pull-request", true},
		{"type:pull-request", true},
		{"is:private review-requested:@me", false},
		{"-is:pr repo:o/r", false},
		{"is:prx", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := hasPRQualifier(tt.query); got != tt.want {
			t.Errorf("hasPRQualifier(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestSearchAddsPRQualifierOnce(t *testing.T) {
	tests := []struct{ in, want string }{
		{"review-requested:@me", "is:pr review-requested:@me"},
		{"is:pr author:octocat", "is:pr author:octocat"},
		{"is:private author:octocat", "is:pr is:private author:octocat"},
		{"-is:pr repo:o/r", "is:pr -is:pr repo:o/r"},
	}
	for _, tt := range tests {
		s := &server{}
		c := newTestClient(t, s)
		if _, err := c.SearchPullRequests(context.Background(), tt.in, github.PageRequest{}); err != nil {
			t.Fatal(err)
		}
		if got := s.Calls()[0].Vars["query"]; got != tt.want {
			t.Errorf("query for %q = %v, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGetPullRequestWithoutTeamScope(t *testing.T) {
	s := &server{}
	s.override = func(c call, w http.ResponseWriter) bool {
		// Only the query that asks for team slugs is refused.
		if c.Op == opPullRequestDetail && strings.Contains(c.Query, "... on Team") {
			s.fixture(w, http.StatusOK, "error_insufficient_scopes.json")
			return true
		}
		if c.Op == opPullRequestDetail {
			// The fallback query cannot read the slug, so GitHub omits it.
			b, err := os.ReadFile("testdata/PullRequestDetail.json")
			if err != nil {
				t.Error(err)
				return true
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(strings.Replace(string(b), `, "slug": "maintainers"`, "", 1)))
			return true
		}
		return false
	}
	c := newTestClient(t, s)
	got, err := c.GetPullRequest(context.Background(), repo, 27)
	if err != nil {
		t.Fatal(err)
	}
	// The team is still listed, with an unknown (empty) slug.
	want := []github.Reviewer{{Name: "octocat"}, {IsTeam: true}}
	if diff := cmp.Diff(want, got.ReviewRequests); diff != "" {
		t.Errorf("reviewers mismatch (-want +got):\n%s", diff)
	}
	var detailCalls int
	for _, c := range s.Calls() {
		if c.Op == opPullRequestDetail {
			detailCalls++
		}
	}
	if detailCalls != 2 {
		t.Errorf("detail query sent %d times, want 2 (one retry)", detailCalls)
	}
}

func TestScopeErrorOnOtherQueriesIsForbidden(t *testing.T) {
	s := &server{}
	s.override = func(_ call, w http.ResponseWriter) bool {
		s.fixture(w, http.StatusOK, "error_insufficient_scopes.json")
		return true
	}
	c := newTestClient(t, s)
	_, err := c.ListReviewThreads(context.Background(), repo, 27, github.PageRequest{})
	if !errors.Is(err, github.ErrForbidden) {
		t.Errorf("want ErrForbidden, got %v", err)
	}
}

func TestGetDiffTooLarge(t *testing.T) {
	s := &server{}
	s.override = func(_ call, w http.ResponseWriter) bool {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		s.fixture(w, http.StatusNotAcceptable, "too_large.json")
		return true
	}
	c := newTestClient(t, s)
	_, err := c.GetDiff(context.Background(), repo, 27)
	if !errors.Is(err, github.ErrDiffTooLarge) {
		t.Errorf("want ErrDiffTooLarge, got %v", err)
	}
}

func TestPartialGraphQLErrorsFail(t *testing.T) {
	s := &server{}
	s.override = func(_ call, w http.ResponseWriter) bool {
		s.fixture(w, http.StatusOK, "error_partial.json")
		return true
	}
	c := newTestClient(t, s)
	page, err := c.ListPullRequests(context.Background(), repo, github.PageRequest{})
	if err == nil {
		t.Fatalf("partial data with errors must fail, got %+v", page)
	}
	if errors.Is(err, github.ErrNotFound) || errors.Is(err, github.ErrForbidden) {
		t.Errorf("unexpected classification: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("no partial items expected, got %v", page.Items)
	}
}

func TestRateLimitBucketsAreIndependent(t *testing.T) {
	graphqlReset, coreReset := "2000", "3000"
	s := &server{}
	s.override = func(c call, w http.ResponseWriter) bool {
		switch c.Op {
		case opPullRequestList:
			w.Header().Set("X-RateLimit-Resource", "graphql")
			w.Header().Set("X-RateLimit-Remaining", "5")
			w.Header().Set("X-RateLimit-Reset", graphqlReset)
			s.fixture(w, http.StatusOK, "error_rate_limited.json")
		case "":
			// A REST response lands in between and must not clobber the
			// GraphQL bucket.
			w.Header().Set("X-RateLimit-Resource", "core")
			w.Header().Set("X-RateLimit-Remaining", "4000")
			w.Header().Set("X-RateLimit-Reset", coreReset)
			s.fixture(w, http.StatusOK, "pull_request.diff")
		default:
			return false
		}
		return true
	}
	c := newTestClient(t, s)
	ctx := context.Background()

	// Prime the graphql bucket, then take a REST response, then fail again
	// from a response carrying no headers at all.
	_, _ = c.ListPullRequests(ctx, repo, github.PageRequest{})
	if _, err := c.GetDiff(ctx, repo, 27); err != nil {
		t.Fatal(err)
	}
	s.override = func(_ call, w http.ResponseWriter) bool {
		s.fixture(w, http.StatusOK, "error_rate_limited.json")
		return true
	}
	_, err := c.ListPullRequests(ctx, repo, github.PageRequest{})
	var rl *github.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("want RateLimitError, got %v", err)
	}
	if !rl.ResetAt.Equal(time.Unix(2_000, 0)) {
		t.Errorf("ResetAt = %v, want the graphql bucket reset", rl.ResetAt.Unix())
	}
}

func TestCollectMoreRejectsRepeatedCursor(t *testing.T) {
	s := &server{}
	s.override = func(c call, w http.ResponseWriter) bool {
		// Every follow-up page claims there is another page with the same cursor.
		if c.Op == "PullRequestFiles" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"conn":{"pageInfo":{"hasNextPage":true,"endCursor":"FILES1"},"nodes":[]}}}}}`))
			return true
		}
		return false
	}
	c := newTestClient(t, s)
	_, err := c.GetPullRequest(context.Background(), repo, 27)
	if err == nil || !strings.Contains(err.Error(), "files:") || !strings.Contains(err.Error(), "repeated cursor") {
		t.Fatalf("want a files pagination error, got %v", err)
	}
}

func TestFollowUpFailureNamesTheConnection(t *testing.T) {
	s := &server{}
	s.override = func(c call, w http.ResponseWriter) bool {
		if c.Op == "PullRequestComments" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"bad gateway"}`))
			return true
		}
		return false
	}
	c := newTestClient(t, s)
	_, err := c.GetPullRequest(context.Background(), repo, 27)
	if err == nil || !strings.Contains(err.Error(), "comments:") {
		t.Fatalf("want an error naming comments, got %v", err)
	}
}
