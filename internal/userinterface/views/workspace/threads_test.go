package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
)

func tc(author, body string, ago time.Duration) github.ReviewComment {
	return github.ReviewComment{
		ID: author + body, Author: author, Body: body, CreatedAt: testNow.Add(-ago),
		Reactions: []github.ReactionSummary{{Content: github.ReactionHeart, Count: 1}},
	}
}

// testThreads covers an open thread, a resolved one and an outdated one on
// internal/sync/client.go (see testdata/pull_request.diff: new line 14 is the added
// `func Sync` line), plus one on a file that has no diff in the pane.
func testThreads() []github.ReviewThread {
	return []github.ReviewThread{
		{
			ID: "t-open", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{tc("bob", "Why retry here?", 5*time.Hour), tc("alice", "Handles flaky links.", 4*time.Hour)},
		},
		{
			ID: "t-resolved", Path: "internal/sync/client.go", Line: 12, DiffSide: github.SideRight, IsResolved: true,
			Comments: []github.ReviewComment{tc("carol", "Nit: unused import.", 30*time.Hour)},
		},
		{
			ID: "t-outdated", Path: "internal/sync/client.go", OriginalLine: 3, DiffSide: github.SideRight, IsOutdated: true,
			Comments: []github.ReviewComment{tc("dave", "This moved.", 70*time.Hour)},
		},
		{
			ID: "t-readme", Path: "README.md", Line: 1, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{tc("erin", "Typo.", time.Hour)},
		},
	}
}

func threadHarness(t *testing.T, w, h int) *harness {
	t.Helper()
	hs := newHarness(t, w, h)
	hs.client.SetReviewThreads(testRepo, 7, testThreads())
	hs.run(hs.v.Init())
	return hs
}

func TestThreadsLoadedAndSorted(t *testing.T) {
	h := threadHarness(t, 100, 40)
	var ids []string
	for _, th := range h.v.threads {
		ids = append(ids, th.ID)
	}
	want := "t-readme,t-outdated,t-resolved,t-open" // by path, then line (outdated by original line)
	if got := strings.Join(ids, ","); got != want {
		t.Errorf("threads = %s, want %s", got, want)
	}
	if h.v.Loading() {
		t.Error("still loading")
	}
}

func TestThreadsPagedLoad(t *testing.T) {
	h := newHarness(t, 100, 40)
	var many []github.ReviewThread
	for i := range 75 {
		many = append(many, github.ReviewThread{ID: fmt.Sprintf("t%02d", i), Path: "README.md", Line: i + 1, Comments: []github.ReviewComment{tc("x", "y", time.Hour)}})
	}
	h.client.SetReviewThreads(testRepo, 7, many)
	h.run(h.v.Init())
	if len(h.v.threads) != 75 {
		t.Errorf("loaded %d threads, want all 75", len(h.v.threads))
	}
}

func TestThreadsInOverview(t *testing.T) {
	h := threadHarness(t, 100, 60)
	h.press("l", "]", "]")
	out := ansi.Strip(h.render())
	for _, want := range []string{
		"Review threads (4)",
		"internal/sync/client.go:14 bob", "Why retry here?", "Handles flaky links.",
		"internal/sync/client.go:12 carol 1 comment · 1d ago [resolved]", "Nit: unused import.",
		"internal/sync/client.go:3 dave", "[outdated]",
		"README.md:1 erin",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overview is missing %q:\n%s", want, out)
		}
	}
}

func TestOverviewWithoutThreads(t *testing.T) {
	h := loaded(t, 100, 40)
	h.press("l", "G")
	if out := ansi.Strip(h.render()); !strings.Contains(out, "Review threads (0)") || !strings.Contains(out, "No review threads.") {
		t.Errorf("empty state missing:\n%s", out)
	}
}

func TestThreadErrorKeepsPullRequestUsable(t *testing.T) {
	h := newHarness(t, 100, 40)
	h.client.SetError(fake.MethodListReviewThreads, errors.New("threads boom"))
	h.run(h.v.Init())
	if h.v.detail == nil || h.v.Loading() {
		t.Fatalf("the pull request should load without threads (detail=%v loading=%v)", h.v.detail != nil, h.v.Loading())
	}
	if err := h.lastErr(); err == nil || !strings.Contains(err.Error(), "review threads") {
		t.Errorf("status bar error = %v", err)
	}
	// A later good refresh brings them in; a failing refresh keeps them.
	h.client.SetError(fake.MethodListReviewThreads, nil)
	h.client.SetReviewThreads(testRepo, 7, testThreads())
	h.press("R")
	if len(h.v.threads) != 4 {
		t.Fatalf("threads after refresh = %d", len(h.v.threads))
	}
	h.client.SetError(fake.MethodListReviewThreads, errors.New("again"))
	h.press("R")
	if len(h.v.threads) != 4 {
		t.Errorf("a failed refresh dropped the threads: %d", len(h.v.threads))
	}
}

func TestStaleThreadsIgnored(t *testing.T) {
	h := loaded(t, 100, 30)
	cur := h.v.seq
	stale := []github.ReviewThread{{ID: "stale", Path: "x"}}
	for name, msg := range map[string]threadsLoadedMsg{
		"old seq":      {owner: h.v, repo: testRepo, number: 7, seq: cur - 1, threads: stale},
		"other number": {owner: h.v, repo: testRepo, number: 8, seq: cur, threads: stale},
		"other view":   {owner: nil, repo: testRepo, number: 7, seq: cur, threads: stale},
	} {
		h.send(msg)
		if len(h.v.threads) != 0 {
			t.Errorf("%s: stale threads applied", name)
		}
	}
}

func (h *harness) selectFile() {
	const path = "internal/sync/client.go"
	h.t.Helper()
	for i, f := range h.v.files() {
		if f.Path == path {
			h.v.selectEntry(i + 1)
			h.press("l")
			return
		}
	}
	h.t.Fatalf("no file %s", path)
}

func TestThreadsInDiff(t *testing.T) {
	h := threadHarness(t, 110, 40)
	h.selectFile()
	out := ansi.Strip(h.render())
	for _, want := range []string{
		"▾ bob 2 comments", "Why retry here?", "Handles flaky links.", "❤️ 1",
		"▸ carol 1 comment · 1d ago [resolved] — Nit: unused import.",
		"[outdated]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Typo.") {
		t.Error("a thread on another file leaked into this diff")
	}
}

func TestThreadKeys(t *testing.T) {
	h := threadHarness(t, 110, 40)
	h.selectFile()
	has := func(s string) bool { return strings.Contains(ansi.Strip(h.render()), s) }

	h.press(")") // first thread in row order: the resolved one on line 12
	if !strings.Contains(h.cursorRow(), "carol") {
		t.Fatalf(") landed on %q", h.cursorRow())
	}
	h.press("z o")
	if !has("Nit: unused import.") || !has("▾ carol") {
		t.Error("z o should open the resolved thread")
	}
	h.press("z c")
	if has("│ carol") {
		t.Error("z c should close it")
	}
	h.press("enter")
	if !has("│ carol") {
		t.Error("enter on a thread header should toggle it")
	}
	h.press("z a")
	if has("│ carol") {
		t.Error("z a should toggle it")
	}

	h.press(")")
	if !strings.Contains(h.cursorRow(), "bob") {
		t.Errorf("second ) landed on %q", h.cursorRow())
	}
	h.press("(")
	if !strings.Contains(h.cursorRow(), "carol") {
		t.Errorf("( landed on %q", h.cursorRow())
	}

	// enter on the commented diff line toggles its thread.
	h.press("(", "k")
	if h.cursorText() == "" || !strings.Contains(h.cursorRow(), "12") {
		t.Fatalf("setup: expected the commented line 12, at %q", h.cursorRow())
	}
	h.press("enter")
	if !has("│ carol") {
		t.Error("enter on a commented line should open its thread")
	}
}

// cursorRow is the screen row holding the diff cursor mark.
func (h *harness) cursorRow() string {
	for _, l := range strings.Split(ansi.Strip(h.render()), "\n") {
		if strings.Contains(l, "││>") {
			return l
		}
	}
	h.t.Fatalf("no cursor row:\n%s", ansi.Strip(h.render()))
	return ""
}

func TestThreadKeysAreRegisteredForSequences(t *testing.T) {
	h := loaded(t, 100, 30)
	want := map[string]bool{")": false, "(": false, "z a": false, "z o": false, "z c": false}
	for _, group := range h.v.FullHelp() {
		for _, b := range group {
			for _, k := range b.Keys() {
				if _, ok := want[k]; ok {
					want[k] = true
				}
			}
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("FullHelp does not register %q", k)
		}
	}
}

func TestEnterInOverviewDoesNothingToThreads(t *testing.T) {
	h := threadHarness(t, 100, 40)
	h.press("l", "enter") // overview focused: must not panic or change selection
	if h.v.sel() != 0 {
		t.Error("enter in the overview changed the selection")
	}
}

func TestGoldenThreads(t *testing.T) {
	t.Run("overview_threads", func(t *testing.T) {
		h := threadHarness(t, 100, 40)
		h.press("l", "]", "]")
		golden.Assert(t, "overview_threads", h.render(), false)
	})
	t.Run("diff_threads", func(t *testing.T) {
		h := threadHarness(t, 100, 40)
		h.selectFile()
		golden.Assert(t, "diff_threads", h.render(), false)
	})
	t.Run("diff_threads_narrow", func(t *testing.T) {
		h := threadHarness(t, 60, 24)
		h.selectFile()
		golden.Assert(t, "diff_threads_narrow", h.render(), false)
	})
}

func overviewThreadsText(h *harness) string {
	h.press("l", "G")
	return ansi.Strip(h.render())
}

func TestOverviewThreadsLoadingState(t *testing.T) {
	h := newHarness(t, 100, 40)
	for _, m := range messages(h.v.Init()) {
		if _, ok := m.(threadsLoadedMsg); !ok { // the thread load is still in flight
			h.send(m)
		}
	}
	out := overviewThreadsText(h)
	if !strings.Contains(out, "Loading review threads…") || strings.Contains(out, "No review threads.") {
		t.Errorf("expected the loading state:\n%s", out)
	}
}

func TestOverviewThreadsErrorState(t *testing.T) {
	h := newHarness(t, 100, 40)
	h.client.SetError(fake.MethodListReviewThreads, errors.New("boom\x1b[2J"))
	h.run(h.v.Init())
	out := overviewThreadsText(h)
	if !strings.Contains(out, "Could not load review threads: ") || !strings.Contains(out, "boom") || strings.Contains(out, "No review threads.") {
		t.Errorf("expected the error state:\n%s", out)
	}
	assertTame(t, "thread error state", h.render())
	// The next load replaces it.
	h.client.SetError(fake.MethodListReviewThreads, nil)
	h.press("R")
	if out := ansi.Strip(h.render()); !strings.Contains(out, "No review threads.") || strings.Contains(out, "Could not load") {
		t.Errorf("expected the empty state after a good load:\n%s", out)
	}
}

func TestOverviewThreadsEmptyState(t *testing.T) {
	h := loaded(t, 100, 40)
	if out := overviewThreadsText(h); !strings.Contains(out, "Review threads (0)") || !strings.Contains(out, "No review threads.") {
		t.Errorf("expected the empty state:\n%s", out)
	}
}
