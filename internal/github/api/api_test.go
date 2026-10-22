package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

var opRE = regexp.MustCompile(`(?:query|mutation)\s+(\w+)`)

// call is one request seen by the fake server.
type call struct {
	Method  string
	Path    string
	Query   string
	Op      string
	Vars    map[string]any
	Accept  string
	Auth    string
	RESTRaw string
}

// server serves testdata fixtures in place of GitHub. It never opens a socket:
// requests are handed to the handler through a RoundTripper.
type server struct {
	t *testing.T
	// override, if it returns true, has produced the response itself.
	override func(c call, w http.ResponseWriter) bool

	mu    sync.Mutex
	calls []call
}

func (s *server) fixture(w http.ResponseWriter, status int, name string) {
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		s.t.Errorf("read fixture: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if filepath.Ext(name) == ".json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	if r.Body != nil {
		raw, _ = io.ReadAll(r.Body)
	}
	c := call{Method: r.Method, Path: r.URL.Path, Accept: r.Header.Get("Accept"), Auth: r.Header.Get("Authorization"), RESTRaw: string(raw)}
	if r.URL.Path == "/graphql" {
		var req struct {
			Query     string
			Variables map[string]any
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			s.t.Errorf("bad graphql request: %v", err)
		}
		c.Vars = req.Variables
		c.Query = req.Query
		if m := opRE.FindStringSubmatch(req.Query); m != nil {
			c.Op = m[1]
		}
	}
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()

	if s.override != nil && s.override(c, w) {
		return
	}
	switch {
	case c.Op != "":
		name := c.Op + ".json"
		if after, _ := c.Vars["after"].(string); after != "" {
			name = c.Op + ".after_" + after + ".json"
		}
		s.fixture(w, http.StatusOK, name)
	case c.Method == "GET" && c.Path == "/repos/MikePors/strigidae/pulls/27":
		s.fixture(w, http.StatusOK, "pr.diff")
	case c.Method == "POST" && c.Path == "/repos/MikePors/strigidae/issues/27/comments":
		s.fixture(w, http.StatusCreated, "issue_comment.json")
	default:
		s.t.Errorf("unexpected request %s %s", c.Method, c.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *server) Calls() []call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]call(nil), s.calls...)
}

type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, r)
	resp := rec.Result()
	resp.Request = r
	return resp, nil
}

func newTestClient(t *testing.T, s *server) *Client {
	t.Helper()
	s.t = t
	c, err := New(Options{
		Host:      "github.com",
		APIHost:   "api.github.test",
		Token:     "tok",
		Transport: handlerTransport{s},
		Now:       func() time.Time { return time.Unix(1_000, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var repo = github.Repo{Owner: "MikePors", Name: "strigidae"}

func sha(n int) string { return fmt.Sprintf("%040x", n*0x1111) }

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestListPullRequestsPagination(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	ctx := context.Background()

	p1, err := c.ListPullRequests(ctx, repo, github.PageRequest{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	want1 := github.Page[github.PullRequest]{
		Total: 3,
		Next:  "Y3Vyc29yOnYyOpHOAAAAAg==",
		Items: []github.PullRequest{
			{
				ID: "PR_kwDOExample01", HeadSHA: sha(1), Repo: repo, Number: 27, Title: "feat(api): add GitHub client layer", Author: "MikePors",
				State: github.PRStateOpen, Draft: true, CommentCount: 2,
				CreatedAt: mustTime(t, "2026-10-18T09:00:00Z"), UpdatedAt: mustTime(t, "2026-10-19T08:30:00Z"),
				URL: "https://github.com/MikePors/strigidae/pull/27", HeadRef: "feat/4-github-client", BaseRef: "main",
			},
			{
				ID: "PR_kwDOExample02", HeadSHA: sha(2), Repo: repo, Number: 26, Title: "chore: add issue forms and PR template", Author: "ghost",
				State: github.PRStateOpen, ReviewDecision: github.ReviewDecisionApproved,
				CreatedAt: mustTime(t, "2026-10-17T12:00:00Z"), UpdatedAt: mustTime(t, "2026-10-18T10:15:00Z"),
				URL: "https://github.com/MikePors/strigidae/pull/26", HeadRef: "chore/3-templates", BaseRef: "main",
			},
		},
	}
	if diff := cmp.Diff(want1, p1); diff != "" {
		t.Errorf("page 1 mismatch (-want +got):\n%s", diff)
	}
	if !p1.HasMore() {
		t.Error("page 1 should have more")
	}

	p2, err := c.ListPullRequests(ctx, repo, github.PageRequest{After: p1.Next})
	if err != nil {
		t.Fatal(err)
	}
	if p2.HasMore() || len(p2.Items) != 1 || p2.Items[0].ReviewDecision != github.ReviewDecisionReviewRequired {
		t.Errorf("unexpected page 2: %+v", p2)
	}

	calls := s.Calls()
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if got := calls[0].Vars["first"]; got != float64(2) {
		t.Errorf("first = %v, want 2", got)
	}
	if got := calls[0].Vars["after"]; got != nil {
		t.Errorf("first page sent after = %v", got)
	}
	if got := calls[1].Vars["first"]; got != float64(github.DefaultPageSize) {
		t.Errorf("default page size = %v", got)
	}
	if got := calls[0].Auth; got != "token tok" {
		t.Errorf("authorization = %q", got)
	}
}

func TestSearchPullRequests(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	page, err := c.SearchPullRequests(context.Background(), "review-requested:@me", github.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Calls()[0].Vars["query"]; got != "is:pr review-requested:@me" {
		t.Errorf("query = %v", got)
	}
	// The empty node (an inaccessible result) is skipped.
	if len(page.Items) != 1 || page.Total != 2 {
		t.Fatalf("unexpected page: %+v", page)
	}
	want := github.PullRequest{
		ID: "PR_kwDOOther01", HeadSHA: sha(1), Repo: github.Repo{Owner: "octo", Name: "widgets"}, Number: 5, Title: "Fix typo in README", Author: "octocat",
		State: github.PRStateOpen, ReviewDecision: github.ReviewDecisionChangesRequested, CommentCount: 4,
		CreatedAt: mustTime(t, "2026-09-01T10:00:00Z"), UpdatedAt: mustTime(t, "2026-09-02T10:00:00Z"),
		URL: "https://github.com/octo/widgets/pull/5", HeadRef: "fix-typo", BaseRef: "main",
	}
	if diff := cmp.Diff(want, page.Items[0]); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestGetPullRequest(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.GetPullRequest(context.Background(), repo, 27)
	if err != nil {
		t.Fatal(err)
	}
	want := github.PullRequestDetail{
		PullRequest: github.PullRequest{
			ID: "PR_kwDOExample01", HeadSHA: sha(1), Repo: repo, Number: 27, Title: "feat(api): add GitHub client layer", Author: "MikePors",
			State: github.PRStateOpen, ReviewDecision: github.ReviewDecisionChangesRequested, CommentCount: 2,
			CreatedAt: mustTime(t, "2026-10-18T09:00:00Z"), UpdatedAt: mustTime(t, "2026-10-19T08:30:00Z"),
			URL: "https://github.com/MikePors/strigidae/pull/27", HeadRef: "feat/4-github-client", BaseRef: "main",
		},
		Body:      "Closes #4\n\n## Summary\nAdds the client layer.\n",
		Additions: 120, Deletions: 7,
		Labels:         []github.Label{{Name: "enhancement", Color: "a2eeef"}},
		ReviewRequests: []github.Reviewer{{Name: "octocat"}, {Name: "maintainers", IsTeam: true}},
		Files: []github.ChangedFile{
			{Path: "internal/github/client.go", Additions: 40, Change: github.FileAdded, Viewed: github.FileViewed},
			{Path: "go.mod", Additions: 3, Deletions: 1, Change: github.FileModified, Viewed: github.FileUnviewed},
			{Path: "docs/old.md", Deletions: 12, Change: github.FileDeleted},
			{Path: "docs/new.md", Additions: 5, Deletions: 5, Change: github.FileRenamed, Viewed: github.FileDismissed},
		},
		Comments: []github.IssueComment{
			{
				ID: "IC_kwDOExample01", Author: "octocat", Body: "Looks good overall.",
				CreatedAt: mustTime(t, "2026-10-18T11:00:00Z"),
				URL:       "https://github.com/MikePors/strigidae/pull/27#issuecomment-1",
				Reactions: []github.ReactionSummary{{Content: github.ReactionThumbsUp, Count: 2, ViewerReacted: true}},
			},
			{
				ID: "IC_kwDOExample02", Author: "ghost", Body: "Thanks!",
				CreatedAt: mustTime(t, "2026-10-18T12:00:00Z"),
				URL:       "https://github.com/MikePors/strigidae/pull/27#issuecomment-2",
			},
			{
				ID: "IC_kwDOExample04", Author: "octocat", Body: "One more thing.",
				CreatedAt: mustTime(t, "2026-10-18T15:00:00Z"),
				URL:       "https://github.com/MikePors/strigidae/pull/27#issuecomment-3",
				Reactions: []github.ReactionSummary{{Content: github.ReactionRocket, Count: 1}},
			},
		},
		Reviews: []github.Review{
			{
				ID: "PRR_kwDOExample01", Author: "octocat", State: github.ReviewChangesRequested,
				Body: "Please add tests.", SubmittedAt: mustTime(t, "2026-10-18T13:00:00Z"),
			},
			// The viewer's draft review is kept; its SubmittedAt is zero.
			{ID: "PRR_kwDOExample02", Author: "MikePors", State: github.ReviewPending},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if n := len(s.Calls()); n != 4 {
		t.Errorf("got %d requests, want 4 (detail + remaining files, comments, reviews)", n)
	}
}

func TestGetDiff(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.GetDiff(context.Background(), repo, 27)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/pr.diff")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("diff mismatch:\n%s", got)
	}
	if a := s.Calls()[0].Accept; a != diffAccept {
		t.Errorf("accept = %q, want %q", a, diffAccept)
	}
}

func TestListReviewThreads(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	ctx := context.Background()

	p1, err := c.ListReviewThreads(ctx, repo, 27, github.PageRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if p1.Next != "THREADS1" || p1.Total != 5 || len(p1.Items) != 2 {
		t.Fatalf("unexpected page 1: next=%q total=%d items=%d", p1.Next, p1.Total, len(p1.Items))
	}
	open := github.ReviewThread{
		ID: "PRRT_kwDOExample01", SubjectType: github.SubjectLine, Path: "internal/github/client.go",
		Line: 12, OriginalLine: 12, StartLine: 10, OriginalStartLine: 10,
		DiffSide: github.SideRight, StartDiffSide: github.SideRight,
		Comments: []github.ReviewComment{
			{
				ID: "PRRC_kwDOExample01", Author: "octocat", Body: "Should this take a context?",
				CreatedAt: mustTime(t, "2026-10-18T13:05:00Z"),
				URL:       "https://github.com/MikePors/strigidae/pull/27#discussion_r1",
				DiffHunk:  "@@ -0,0 +1,20 @@\n+package github\n",
				Reactions: []github.ReactionSummary{{Content: github.ReactionEyes, Count: 1, ViewerReacted: true}},
			},
			{
				ID: "PRRC_kwDOExample02", Author: "MikePors", Body: "Yes, fixed.",
				CreatedAt: mustTime(t, "2026-10-18T14:00:00Z"),
				URL:       "https://github.com/MikePors/strigidae/pull/27#discussion_r2",
				DiffHunk:  "@@ -0,0 +1,20 @@\n+package github\n",
			},
		},
	}
	if diff := cmp.Diff(open, p1.Items[0]); diff != "" {
		t.Errorf("open thread mismatch (-want +got):\n%s", diff)
	}
	resolved := p1.Items[1]
	if !resolved.IsResolved || !resolved.IsOutdated || resolved.ResolvedBy != "MikePors" ||
		resolved.Line != 0 || resolved.OriginalLine != 5 || resolved.DiffSide != github.SideLeft ||
		resolved.Comments[0].Author != "ghost" {
		t.Errorf("unexpected resolved+outdated thread: %+v", resolved)
	}

	p2, err := c.ListReviewThreads(ctx, repo, 27, github.PageRequest{After: p1.Next})
	if err != nil {
		t.Fatal(err)
	}
	if p2.HasMore() || len(p2.Items) != 3 || !p2.Items[0].IsOutdated || p2.Items[0].IsResolved {
		t.Fatalf("unexpected page 2: %+v", p2)
	}
	// A file-level thread has no lines but is not outdated.
	file := p2.Items[1]
	if file.SubjectType != github.SubjectFile || file.IsOutdated || file.Line != 0 || file.OriginalLine != 0 {
		t.Errorf("unexpected file-level thread: %+v", file)
	}
	// A range can start on the removed side and end on the added side, and
	// a pending draft comment is flagged.
	rng := p2.Items[2]
	if rng.StartDiffSide != github.SideLeft || rng.DiffSide != github.SideRight || rng.StartLine != 12 || rng.Line != 30 ||
		len(rng.Comments) != 1 || !rng.Comments[0].Pending {
		t.Errorf("unexpected cross-side range thread: %+v", rng)
	}
}

func TestReplyToThread(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.ReplyToThread(context.Background(), "PRRT_kwDOExample01", "Done.")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "PRRC_kwDOExample05" || got.Body != "Done." || got.Author != "MikePors" {
		t.Errorf("unexpected comment: %+v", got)
	}
	vars := s.Calls()[0].Vars
	if vars["threadId"] != "PRRT_kwDOExample01" || vars["body"] != "Done." {
		t.Errorf("unexpected variables: %v", vars)
	}
}

func TestAddComment(t *testing.T) {
	s := &server{}
	c := newTestClient(t, s)
	got, err := c.AddComment(context.Background(), repo, 27, "Ship it.")
	if err != nil {
		t.Fatal(err)
	}
	want := github.IssueComment{
		ID: "IC_kwDOExample03", Author: "MikePors", Body: "Ship it.",
		CreatedAt: mustTime(t, "2026-10-19T09:30:00Z"),
		URL:       "https://github.com/MikePors/strigidae/pull/27#issuecomment-2001",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if raw := s.Calls()[0].RESTRaw; raw != `{"body":"Ship it."}` {
		t.Errorf("request body = %s", raw)
	}
}

func TestErrors(t *testing.T) {
	resetAt := time.Unix(2_000, 0)
	jsonError := func(w http.ResponseWriter, status int, headers map[string]string, body string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
	fixtureError := func(s *server, name string, headers map[string]string) func(call, http.ResponseWriter) bool {
		return func(_ call, w http.ResponseWriter) bool {
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			s.fixture(w, http.StatusOK, name)
			return true
		}
	}

	tests := []struct {
		name     string
		override func(s *server) func(call, http.ResponseWriter) bool
		run      func(c *Client) error
		check    func(t *testing.T, err error)
	}{
		{
			name: "graphql not found",
			override: func(s *server) func(call, http.ResponseWriter) bool {
				return fixtureError(s, "error_not_found.json", nil)
			},
			run: func(c *Client) error { _, err := c.GetPullRequest(context.Background(), repo, 9999); return err },
			check: func(t *testing.T, err error) {
				if !errors.Is(err, github.ErrNotFound) {
					t.Errorf("want ErrNotFound, got %v", err)
				}
			},
		},
		{
			name: "graphql rate limited with points remaining uses graphql bucket reset",
			override: func(s *server) func(call, http.ResponseWriter) bool {
				return fixtureError(s, "error_rate_limited.json", map[string]string{
					"X-RateLimit-Resource": "graphql", "X-RateLimit-Remaining": "5", "X-RateLimit-Reset": strconv.FormatInt(resetAt.Unix(), 10),
				})
			},
			run: func(c *Client) error {
				_, err := c.ListPullRequests(context.Background(), repo, github.PageRequest{})
				return err
			},
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) {
					t.Fatalf("want RateLimitError, got %v", err)
				}
				if !rl.ResetAt.Equal(resetAt) {
					t.Errorf("ResetAt = %v, want %v", rl.ResetAt, resetAt)
				}
				if d := rl.RetryAfter(time.Unix(1_000, 0)); d != 1000*time.Second {
					t.Errorf("RetryAfter = %v", d)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{}
			s.override = tt.override(s)
			c := newTestClient(t, s)
			tt.check(t, tt.run(c))
		})
	}

	httpTests := []struct {
		name    string
		status  int
		headers map[string]string
		body    string
		check   func(t *testing.T, err error)
	}{
		{
			name: "404", status: 404, body: `{"message":"Not Found"}`,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, github.ErrNotFound) {
					t.Errorf("want ErrNotFound, got %v", err)
				}
			},
		},
		{
			name: "401", status: 401, body: `{"message":"Bad credentials"}`,
			check: func(t *testing.T, err error) {
				if !errors.Is(err, github.ErrNotAuthenticated) {
					t.Errorf("want ErrNotAuthenticated, got %v", err)
				}
			},
		},
		{
			name: "403 permission", status: 403, body: `{"message":"Resource not accessible by integration"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.Is(err, github.ErrForbidden) || errors.As(err, &rl) {
					t.Errorf("want ErrForbidden only, got %v", err)
				}
			},
		},
		{
			name: "403 primary rate limit", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "2000"},
			body:    `{"message":"API rate limit exceeded"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) || !rl.ResetAt.Equal(resetAt) {
					t.Errorf("want RateLimitError reset at %v, got %v", resetAt, err)
				}
			},
		},
		{
			name: "403 secondary rate limit", status: 403,
			headers: map[string]string{"Retry-After": "60"},
			body:    `{"message":"You have exceeded a secondary rate limit"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) || !rl.ResetAt.Equal(time.Unix(1_060, 0)) {
					t.Errorf("want RateLimitError reset at 1060, got %v", err)
				}
			},
		},
		{
			// The primary bucket still has requests; its reset must not be used.
			name: "403 secondary rate limit without Retry-After", status: 403,
			headers: map[string]string{"X-RateLimit-Remaining": "4200", "X-RateLimit-Reset": "4600"},
			body:    `{"message":"You have exceeded a secondary rate limit"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) || !rl.ResetAt.Equal(time.Unix(1_060, 0)) {
					t.Errorf("want RateLimitError reset at now+1m, got %v", err)
				}
			},
		},
		{
			name: "403 secondary rate limit with Retry-After date", status: 403,
			headers: map[string]string{"Retry-After": time.Unix(1_300, 0).UTC().Format(http.TimeFormat)},
			body:    `{"message":"You have exceeded a secondary rate limit"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) || !rl.ResetAt.Equal(time.Unix(1_300, 0)) {
					t.Errorf("want RateLimitError reset at 1300, got %v", err)
				}
			},
		},
		{
			name: "429 without headers", status: 429, body: `{"message":"slow down"}`,
			check: func(t *testing.T, err error) {
				var rl *github.RateLimitError
				if !errors.As(err, &rl) || !rl.ResetAt.Equal(time.Unix(1_060, 0)) {
					t.Errorf("want RateLimitError reset at now+1m, got %v", err)
				}
			},
		},
	}
	for _, tt := range httpTests {
		t.Run("http "+tt.name, func(t *testing.T) {
			s := &server{override: func(_ call, w http.ResponseWriter) bool {
				jsonError(w, tt.status, tt.headers, tt.body)
				return true
			}}
			c := newTestClient(t, s)
			_, err := c.AddComment(context.Background(), repo, 27, "x")
			tt.check(t, err)
			// GraphQL goes through the same mapping.
			_, err = c.ListPullRequests(context.Background(), repo, github.PageRequest{})
			tt.check(t, err)
		})
	}
}

func TestNewWithoutToken(t *testing.T) {
	_, err := New(Options{
		Host:        "github.com",
		TokenSource: func(string) string { return "" },
		Transport:   handlerTransport{http.NotFoundHandler()},
	})
	if !errors.Is(err, github.ErrNotAuthenticated) {
		t.Fatalf("want ErrNotAuthenticated, got %v", err)
	}
	if got := err.Error(); !regexp.MustCompile(`gh auth login`).MatchString(got) {
		t.Errorf("error should mention gh auth login: %q", got)
	}
}

func TestNewUsesTokenSource(t *testing.T) {
	var asked string
	s := &server{t: t}
	c, err := New(Options{
		Host:    "ghe.example.test",
		APIHost: "api.ghe.example.test",
		TokenSource: func(host string) string {
			asked = host
			return "from-source"
		},
		Transport: handlerTransport{s},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "ghe.example.test" || c.Host() != "ghe.example.test" {
		t.Errorf("token looked up for %q, client host %q", asked, c.Host())
	}
}

func TestSetFileViewed(t *testing.T) {
	tests := []struct {
		name   string
		viewed bool
		op     string
		field  string
	}{
		{"mark", true, "MarkFileViewed", "markFileAsViewed"},
		{"unmark", false, "UnmarkFileViewed", "unmarkFileAsViewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &server{}
			c := newTestClient(t, s)
			if err := c.SetFileViewed(context.Background(), "PR_kwDOExample01", "go.mod", tt.viewed); err != nil {
				t.Fatal(err)
			}
			got := s.Calls()[0]
			if got.Op != tt.op || !strings.Contains(got.Query, tt.field+"(input:") {
				t.Errorf("op = %q, query = %q", got.Op, got.Query)
			}
			if got.Vars["pullRequestId"] != "PR_kwDOExample01" || got.Vars["path"] != "go.mod" {
				t.Errorf("unexpected variables: %v", got.Vars)
			}
		})
	}
}

func TestSetFileViewedError(t *testing.T) {
	s := &server{}
	s.override = func(_ call, w http.ResponseWriter) bool {
		s.fixture(w, http.StatusOK, "error_not_found.json")
		return true
	}
	c := newTestClient(t, s)
	err := c.SetFileViewed(context.Background(), "PR_x", "go.mod", true)
	if !errors.Is(err, github.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
