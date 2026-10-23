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

func TestHasPullRequestQualifier(test *testing.T) {
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
	for _, testCase := range tests {
		if got := hasPullRequestQualifier(testCase.query); got != testCase.want {
			test.Errorf("hasPRQualifier(%q) = %v, want %v", testCase.query, got, testCase.want)
		}
	}
}

func TestSearchAddsPullRequestQualifierOnce(test *testing.T) {
	tests := []struct{ in, want string }{
		{"review-requested:@me", "is:pr review-requested:@me"},
		{"is:pr author:octocat", "is:pr author:octocat"},
		{"is:private author:octocat", "is:pr is:private author:octocat"},
		{"-is:pr repo:o/r", "is:pr -is:pr repo:o/r"},
	}
	for _, testCase := range tests {
		testServer := &server{}
		client := newTestClient(test, testServer)
		if _, err := client.SearchPullRequests(context.Background(), testCase.in, github.PageRequest{}); err != nil {
			test.Fatal(err)
		}
		if got := testServer.Calls()[0].Variables["query"]; got != testCase.want {
			test.Errorf("query for %q = %v, want %q", testCase.in, got, testCase.want)
		}
	}
}

func TestGetPullRequestWithoutTeamScope(test *testing.T) {
	testServer := &server{}
	testServer.override = func(call call, writer http.ResponseWriter) bool {
		// Only the query that asks for team slugs is refused.
		if call.Operation == operationPullRequestDetail && strings.Contains(call.Query, "... on Team") {
			testServer.fixture(writer, http.StatusOK, "error_insufficient_scopes.json")
			return true
		}
		if call.Operation == operationPullRequestDetail {
			// The fallback query cannot read the slug, so GitHub omits it.
			bytes, err := os.ReadFile("testdata/PullRequestDetail.json")
			if err != nil {
				test.Error(err)
				return true
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = writer.Write([]byte(strings.Replace(string(bytes), `, "slug": "maintainers"`, "", 1)))
			return true
		}
		return false
	}
	client := newTestClient(test, testServer)
	got, err := client.GetPullRequest(context.Background(), repository, 27)
	if err != nil {
		test.Fatal(err)
	}
	// The team is still listed, with an unknown (empty) slug.
	want := []github.Reviewer{{Name: "octocat"}, {IsTeam: true}}
	if diff := cmp.Diff(want, got.ReviewRequests); diff != "" {
		test.Errorf("reviewers mismatch (-want +got):\n%s", diff)
	}
	var detailCalls int
	for _, call := range testServer.Calls() {
		if call.Operation == operationPullRequestDetail {
			detailCalls++
		}
	}
	if detailCalls != 2 {
		test.Errorf("detail query sent %d times, want 2 (one retry)", detailCalls)
	}
}

func TestScopeErrorOnOtherQueriesIsForbidden(test *testing.T) {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, "error_insufficient_scopes.json")
		return true
	}
	client := newTestClient(test, testServer)
	_, err := client.ListReviewThreads(context.Background(), repository, 27, github.PageRequest{})
	if !errors.Is(err, github.ErrForbidden) {
		test.Errorf("want ErrForbidden, got %v", err)
	}
}

func TestGetDiffTooLarge(test *testing.T) {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		testServer.fixture(writer, http.StatusNotAcceptable, "too_large.json")
		return true
	}
	client := newTestClient(test, testServer)
	_, err := client.GetDiff(context.Background(), repository, 27)
	if !errors.Is(err, github.ErrDiffTooLarge) {
		test.Errorf("want ErrDiffTooLarge, got %v", err)
	}
}

func TestPartialGraphQLErrorsFail(test *testing.T) {
	testServer := &server{}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, "error_partial.json")
		return true
	}
	client := newTestClient(test, testServer)
	page, err := client.ListPullRequests(context.Background(), repository, github.PageRequest{})
	if err == nil {
		test.Fatalf("partial data with errors must fail, got %+v", page)
	}
	if errors.Is(err, github.ErrNotFound) || errors.Is(err, github.ErrForbidden) {
		test.Errorf("unexpected classification: %v", err)
	}
	if len(page.Items) != 0 {
		test.Errorf("no partial items expected, got %v", page.Items)
	}
}

func TestRateLimitBucketsAreIndependent(test *testing.T) {
	graphqlReset, coreReset := "2000", "3000"
	testServer := &server{}
	testServer.override = func(call call, writer http.ResponseWriter) bool {
		switch call.Operation {
		case operationPullRequestList:
			writer.Header().Set("X-RateLimit-Resource", "graphql")
			writer.Header().Set("X-RateLimit-Remaining", "5")
			writer.Header().Set("X-RateLimit-Reset", graphqlReset)
			testServer.fixture(writer, http.StatusOK, "error_rate_limited.json")
		case "":
			// A REST response lands in between and must not clobber the
			// GraphQL bucket.
			writer.Header().Set("X-RateLimit-Resource", "core")
			writer.Header().Set("X-RateLimit-Remaining", "4000")
			writer.Header().Set("X-RateLimit-Reset", coreReset)
			testServer.fixture(writer, http.StatusOK, "pull_request.diff")
		default:
			return false
		}
		return true
	}
	client := newTestClient(test, testServer)
	ctx := context.Background()

	// Prime the graphql bucket, then take a REST response, then fail again
	// from a response carrying no headers at all.
	_, _ = client.ListPullRequests(ctx, repository, github.PageRequest{})
	if _, err := client.GetDiff(ctx, repository, 27); err != nil {
		test.Fatal(err)
	}
	testServer.override = func(_ call, writer http.ResponseWriter) bool {
		testServer.fixture(writer, http.StatusOK, "error_rate_limited.json")
		return true
	}
	_, err := client.ListPullRequests(ctx, repository, github.PageRequest{})
	var rateLimitError *github.RateLimitError
	if !errors.As(err, &rateLimitError) {
		test.Fatalf("want RateLimitError, got %v", err)
	}
	if !rateLimitError.ResetAt.Equal(time.Unix(2_000, 0)) {
		test.Errorf("ResetAt = %v, want the graphql bucket reset", rateLimitError.ResetAt.Unix())
	}
}

func TestCollectMoreRejectsRepeatedCursor(test *testing.T) {
	testServer := &server{}
	testServer.override = func(call call, writer http.ResponseWriter) bool {
		// Every follow-up page claims there is another page with the same cursor.
		if call.Operation == "PullRequestFiles" {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = writer.Write([]byte(`{"data":{"repository":{"pullRequest":{"conn":{"pageInfo":{"hasNextPage":true,"endCursor":"FILES1"},"nodes":[]}}}}}`))
			return true
		}
		return false
	}
	client := newTestClient(test, testServer)
	_, err := client.GetPullRequest(context.Background(), repository, 27)
	if err == nil || !strings.Contains(err.Error(), "files:") || !strings.Contains(err.Error(), "repeated cursor") {
		test.Fatalf("want a files pagination error, got %v", err)
	}
}

func TestFollowUpFailureNamesTheConnection(test *testing.T) {
	testServer := &server{}
	testServer.override = func(call call, writer http.ResponseWriter) bool {
		if call.Operation == "PullRequestComments" {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			writer.WriteHeader(http.StatusBadGateway)
			_, _ = writer.Write([]byte(`{"message":"bad gateway"}`))
			return true
		}
		return false
	}
	client := newTestClient(test, testServer)
	_, err := client.GetPullRequest(context.Background(), repository, 27)
	if err == nil || !strings.Contains(err.Error(), "comments:") {
		test.Fatalf("want an error naming comments, got %v", err)
	}
}
