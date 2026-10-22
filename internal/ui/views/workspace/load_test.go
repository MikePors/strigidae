package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

func TestLoad(t *testing.T) {
	h := newHarness(t, 100, 30)
	if got := h.v.Title(); got != "#7" {
		t.Errorf("Title = %q", got)
	}
	cmd := h.v.Init()
	if !h.v.Loading() {
		t.Error("should be loading after Init")
	}
	h.run(cmd)
	if h.v.Loading() {
		t.Error("should have stopped loading")
	}
	if h.v.detail == nil || h.v.diff.Raw == "" {
		t.Fatalf("detail=%v diff=%q", h.v.detail, h.v.diff.Raw)
	}
	var calls []string
	for _, c := range h.client.Calls() {
		calls = append(calls, c.Method)
	}
	if diff := cmp.Diff([]string{fake.MethodGetPullRequest, fake.MethodGetDiff, fake.MethodListReviewThreads}, calls); diff != "" {
		t.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		notFound bool
	}{
		{"not found", fmt.Errorf("load: %w", github.ErrNotFound), true},
		{"other", errors.New("boom"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, 100, 30)
			h.client.SetError(fake.MethodGetPullRequest, tt.err)
			h.run(h.v.Init())
			if h.v.notFound != tt.notFound || h.v.err == nil {
				t.Errorf("notFound=%v err=%v", h.v.notFound, h.v.err)
			}
			if !errors.Is(h.lastErr(), tt.err) {
				t.Errorf("status bar error = %v, want %v", h.lastErr(), tt.err)
			}
			if h.v.Loading() == false && h.v.diffPending {
				t.Error("inconsistent loading state")
			}
		})
	}
}

func TestRefreshKeepsContentOnError(t *testing.T) {
	h := loaded(t, 100, 30)
	h.press("j", "j") // select the second file
	path := h.v.selectedPath()
	h.client.SetError(fake.MethodGetPullRequest, errors.New("flaky"))
	h.press("R")
	if h.v.detail == nil || h.v.err != nil {
		t.Error("refresh failure must keep the loaded content")
	}
	if h.lastErr() == nil {
		t.Error("refresh failure should reach the status bar")
	}
	h.client.SetError(fake.MethodGetPullRequest, nil)
	h.press("R")
	if h.v.selectedPath() != path {
		t.Errorf("selection after refresh = %q, want %q", h.v.selectedPath(), path)
	}
	n := 0
	for _, c := range h.client.Calls() {
		if c.Method == fake.MethodGetPullRequest {
			n++
		}
	}
	if n != 3 {
		t.Errorf("GetPullRequest calls = %d, want 3", n)
	}
}

func TestStaleResultsIgnored(t *testing.T) {
	h := loaded(t, 100, 30)
	other := testDetail()
	other.Title = "stale"
	cur := h.v.seq
	for name, msg := range map[string]prLoadedMsg{
		"old seq":      {owner: h.v, repo: testRepo, number: 7, seq: cur - 1, detail: other},
		"other number": {owner: h.v, repo: testRepo, number: 8, seq: cur, detail: other},
		"other repo":   {owner: h.v, repo: github.Repo{Owner: "x", Name: "y"}, number: 7, seq: cur, detail: other},
	} {
		h.send(msg)
		if h.v.detail.Title == "stale" {
			t.Errorf("%s: stale result applied", name)
		}
	}
	h.send(diffLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: cur - 1, raw: "stale"})
	if h.v.diff.Raw == "stale" {
		t.Error("stale diff applied")
	}
	h.press("R")
	prev := h.v.seq - 1
	h.send(prLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: prev, detail: other})
	if h.v.detail.Title == "stale" {
		t.Error("result of a superseded request applied")
	}
}

func TestDiffRefreshFailureKeepsDiff(t *testing.T) {
	h := loaded(t, 100, 30)
	before := h.v.diff.Raw
	h.client.SetError(fake.MethodGetDiff, errors.New("blip"))
	h.press("R")
	if h.v.diff.Raw != before || h.v.diff.Loading {
		t.Errorf("diff after failed refresh = %+v, want Raw kept and not loading", h.v.diff)
	}
	if h.lastErr() == nil {
		t.Error("diff failure should reach the status bar")
	}
	// A too-large diff clears the text.
	h.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
	h.press("R")
	if h.v.diff.Raw != "" || !errors.Is(h.v.diff.Err, github.ErrDiffTooLarge) {
		t.Errorf("too-large diff = %+v", h.v.diff)
	}
}

// TestReopenedWorkspaceIgnoresOldResults: open #N, close it before it loads,
// open #N again. Both instances count requests from 1, so only the view
// identity tells their results apart.
func TestReopenedWorkspaceIgnoresOldResults(t *testing.T) {
	c := fake.New()
	old := testDetail()
	old.Title = "old snapshot"
	c.SetPullRequest(old)
	c.SetDiff(testRepo, 7, readDiff(t))
	opts := []Option{WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true))}

	first := New(c, testRepo, 7, opts...)
	oldResults := messages(first.Init()) // still "in flight" when it is closed

	fresh := testDetail()
	fresh.Title = "fresh data"
	c.SetPullRequest(fresh)
	second := New(c, testRepo, 7, opts...)
	newResults := messages(second.Init())

	for _, m := range oldResults {
		second.Update(m)
	}
	if second.detail != nil || !second.Loading() {
		t.Fatalf("results of the closed workspace were accepted: detail=%v loading=%v", second.detail != nil, second.Loading())
	}
	feed(second, newResults)
	if second.detail == nil || second.detail.Title != "fresh data" || second.Loading() {
		t.Errorf("own results not applied: detail=%v loading=%v", second.detail, second.Loading())
	}
	// And the other way round: the new workspace's results mean nothing to the old one.
	for _, m := range newResults {
		first.Update(m)
	}
	if first.detail != nil {
		t.Error("the closed workspace accepted a newer instance's result")
	}
}

// malformedDiff has a good first file and a second whose hunk runs past its
// line counts.
func malformedDiff(t *testing.T) string {
	t.Helper()
	good, _, _ := strings.Cut(readDiff(t), "diff --git a/internal/sync/retry.go")
	return good + "diff --git a/internal/sync/retry.go b/internal/sync/retry.go\n" +
		"new file mode 100644\n--- /dev/null\n+++ b/internal/sync/retry.go\n" +
		"@@ -0,0 +1,1 @@\n+one\n+two\n+three\n"
}

func TestPartialDiffStillShowsParsedFiles(t *testing.T) {
	h := newHarness(t, 100, 24)
	h.client.SetDiff(testRepo, 7, malformedDiff(t))
	h.run(h.v.Init())

	h.press("}") // client.go parsed fine
	if out := ansi.Strip(h.render()); !strings.Contains(out, "func (c *Client) Sync") {
		t.Errorf("a file that parsed should show its diff:\n%s", out)
	}
	h.press("}") // retry.go is the malformed one: what parsed before the problem shows
	if out := ansi.Strip(h.render()); !strings.Contains(out, "+one") {
		t.Errorf("the partly parsed file should show what parsed:\n%s", out)
	}
	h.press("}") // legacy.go is not in the diff at all
	out := ansi.Strip(h.render())
	if !strings.Contains(out, "No diff available for this file.") || !strings.Contains(out, "could not be fully parsed") {
		t.Errorf("the missing file should say so:\n%s", out)
	}

	var errs []string
	for _, m := range h.msgs {
		if e, ok := m.(ui.ErrorMsg); ok {
			errs = append(errs, e.Err.Error())
		}
	}
	if len(errs) != 1 {
		t.Fatalf("status bar errors = %q, want exactly one", errs)
	}
	if n := strings.Count(errs[0], "parse diff"); n != 1 || !strings.HasPrefix(errs[0], "acme/widgets#7: ") {
		t.Errorf("status bar text = %q", errs[0])
	}
}
