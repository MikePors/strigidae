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
		Identifier: author + body, Author: author, Body: body, CreatedAt: testNow.Add(-ago),
		Reactions: []github.ReactionSummary{{Content: github.ReactionHeart, Count: 1}},
	}
}

// testThreads covers an open thread, a resolved one and an outdated one on
// internal/sync/client.go (see testdata/pull_request.diff: new line 14 is the added
// `func Sync` line), plus one on a file that has no diff in the pane.
func testThreads() []github.ReviewThread {
	return []github.ReviewThread{
		{
			Identifier: "t-open", Path: "internal/sync/client.go", Line: 14, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{tc("bob", "Why retry here?", 5*time.Hour), tc("alice", "Handles flaky links.", 4*time.Hour)},
		},
		{
			Identifier: "t-resolved", Path: "internal/sync/client.go", Line: 12, DiffSide: github.SideRight, IsResolved: true,
			Comments: []github.ReviewComment{tc("carol", "Nit: unused import.", 30*time.Hour)},
		},
		{
			Identifier: "t-outdated", Path: "internal/sync/client.go", OriginalLine: 3, DiffSide: github.SideRight, IsOutdated: true,
			Comments: []github.ReviewComment{tc("dave", "This moved.", 70*time.Hour)},
		},
		{
			Identifier: "t-readme", Path: "README.md", Line: 1, DiffSide: github.SideRight,
			Comments: []github.ReviewComment{tc("erin", "Typo.", time.Hour)},
		},
	}
}

func threadHarness(test *testing.T, width, height int) *harness {
	test.Helper()
	otherHarness := newHarness(test, width, height)
	otherHarness.client.SetReviewThreads(testRepository, 7, testThreads())
	otherHarness.run(otherHarness.view.Init())
	return otherHarness
}

func TestThreadsLoadedAndSorted(test *testing.T) {
	harness := threadHarness(test, 100, 40)
	var identifiers []string
	for _, thread := range harness.view.threads {
		identifiers = append(identifiers, thread.Identifier)
	}
	want := "t-readme,t-outdated,t-resolved,t-open" // by path, then line (outdated by original line)
	if got := strings.Join(identifiers, ","); got != want {
		test.Errorf("threads = %s, want %s", got, want)
	}
	if harness.view.Loading() {
		test.Error("still loading")
	}
}

func TestThreadsPagedLoad(test *testing.T) {
	harness := newHarness(test, 100, 40)
	var many []github.ReviewThread
	for i := range 75 {
		many = append(many, github.ReviewThread{Identifier: fmt.Sprintf("t%02d", i), Path: "README.md", Line: i + 1, Comments: []github.ReviewComment{tc("x", "y", time.Hour)}})
	}
	harness.client.SetReviewThreads(testRepository, 7, many)
	harness.run(harness.view.Init())
	if len(harness.view.threads) != 75 {
		test.Errorf("loaded %d threads, want all 75", len(harness.view.threads))
	}
}

func TestThreadsInOverview(test *testing.T) {
	harness := threadHarness(test, 100, 60)
	harness.press("l", "]", "]")
	out := ansi.Strip(harness.render())
	for _, want := range []string{
		"Review threads (4)",
		"internal/sync/client.go:14 bob", "Why retry here?", "Handles flaky links.",
		"internal/sync/client.go:12 carol 1 comment · 1d ago [resolved]", "Nit: unused import.",
		"internal/sync/client.go:3 dave", "[outdated]",
		"README.md:1 erin",
	} {
		if !strings.Contains(out, want) {
			test.Errorf("overview is missing %q:\n%s", want, out)
		}
	}
}

func TestOverviewWithoutThreads(test *testing.T) {
	harness := loaded(test, 100, 40)
	harness.press("l", "G")
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "Review threads (0)") || !strings.Contains(out, "No review threads.") {
		test.Errorf("empty state missing:\n%s", out)
	}
}

func TestThreadErrorKeepsPullRequestUsable(test *testing.T) {
	harness := newHarness(test, 100, 40)
	harness.client.SetError(fake.MethodListReviewThreads, errors.New("threads boom"))
	harness.run(harness.view.Init())
	if harness.view.detail == nil || harness.view.Loading() {
		test.Fatalf("the pull request should load without threads (detail=%v loading=%v)", harness.view.detail != nil, harness.view.Loading())
	}
	if err := harness.lastErr(); err == nil || !strings.Contains(err.Error(), "review threads") {
		test.Errorf("status bar error = %v", err)
	}
	// A later good refresh brings them in; a failing refresh keeps them.
	harness.client.SetError(fake.MethodListReviewThreads, nil)
	harness.client.SetReviewThreads(testRepository, 7, testThreads())
	harness.press("R")
	if len(harness.view.threads) != 4 {
		test.Fatalf("threads after refresh = %d", len(harness.view.threads))
	}
	harness.client.SetError(fake.MethodListReviewThreads, errors.New("again"))
	harness.press("R")
	if len(harness.view.threads) != 4 {
		test.Errorf("a failed refresh dropped the threads: %d", len(harness.view.threads))
	}
}

func TestStaleThreadsIgnored(test *testing.T) {
	harness := loaded(test, 100, 30)
	current := harness.view.sequence
	stale := []github.ReviewThread{{Identifier: "stale", Path: "x"}}
	for name, message := range map[string]threadsLoadedMessage{
		"old seq":      {owner: harness.view, repository: testRepository, number: 7, sequence: current - 1, threads: stale},
		"other number": {owner: harness.view, repository: testRepository, number: 8, sequence: current, threads: stale},
		"other view":   {owner: nil, repository: testRepository, number: 7, sequence: current, threads: stale},
	} {
		harness.send(message)
		if len(harness.view.threads) != 0 {
			test.Errorf("%s: stale threads applied", name)
		}
	}
}

func (harness *harness) selectFile() {
	const path = "internal/sync/client.go"
	harness.test.Helper()
	for i, file := range harness.view.files() {
		if file.Path == path {
			harness.view.selectEntry(i + 1)
			harness.press("l")
			return
		}
	}
	harness.test.Fatalf("no file %s", path)
}

func TestThreadsInDiff(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	out := ansi.Strip(harness.render())
	for _, want := range []string{
		"▾ bob 2 comments", "Why retry here?", "Handles flaky links.", "❤️ 1",
		"▸ carol 1 comment · 1d ago [resolved] — Nit: unused import.",
		"[outdated]",
	} {
		if !strings.Contains(out, want) {
			test.Errorf("diff is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Typo.") {
		test.Error("a thread on another file leaked into this diff")
	}
}

func TestThreadKeys(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	has := func(text string) bool { return strings.Contains(ansi.Strip(harness.render()), text) }

	harness.press(")") // first thread in row order: the resolved one on line 12
	if !strings.Contains(harness.cursorRow(), "carol") {
		test.Fatalf(") landed on %q", harness.cursorRow())
	}
	harness.press("z o")
	if !has("Nit: unused import.") || !has("▾ carol") {
		test.Error("z o should open the resolved thread")
	}
	harness.press("z c")
	if has("│ carol") {
		test.Error("z c should close it")
	}
	harness.press("enter")
	if !has("│ carol") {
		test.Error("enter on a thread header should toggle it")
	}
	harness.press("z a")
	if has("│ carol") {
		test.Error("z a should toggle it")
	}

	harness.press(")")
	if !strings.Contains(harness.cursorRow(), "bob") {
		test.Errorf("second ) landed on %q", harness.cursorRow())
	}
	harness.press("(")
	if !strings.Contains(harness.cursorRow(), "carol") {
		test.Errorf("( landed on %q", harness.cursorRow())
	}

	// enter on the commented diff line toggles its thread.
	harness.press("(", "k")
	if harness.cursorText() == "" || !strings.Contains(harness.cursorRow(), "12") {
		test.Fatalf("setup: expected the commented line 12, at %q", harness.cursorRow())
	}
	harness.press("enter")
	if !has("│ carol") {
		test.Error("enter on a commented line should open its thread")
	}
}

// cursorRow is the screen row holding the diff cursor mark.
func (harness *harness) cursorRow() string {
	for _, line := range strings.Split(ansi.Strip(harness.render()), "\n") {
		if strings.Contains(line, "││>") {
			return line
		}
	}
	harness.test.Fatalf("no cursor row:\n%s", ansi.Strip(harness.render()))
	return ""
}

func TestThreadKeysAreRegisteredForSequences(test *testing.T) {
	harness := loaded(test, 100, 30)
	want := map[string]bool{")": false, "(": false, "z a": false, "z o": false, "z c": false}
	for _, group := range harness.view.FullHelp() {
		for _, binding := range group {
			for _, k := range binding.Keys() {
				if _, ok := want[k]; ok {
					want[k] = true
				}
			}
		}
	}
	for k, ok := range want {
		if !ok {
			test.Errorf("FullHelp does not register %q", k)
		}
	}
}

func TestEnterInOverviewDoesNothingToThreads(test *testing.T) {
	harness := threadHarness(test, 100, 40)
	harness.press("l", "enter") // overview focused: must not panic or change selection
	if harness.view.selection() != 0 {
		test.Error("enter in the overview changed the selection")
	}
}

func TestGoldenThreads(test *testing.T) {
	test.Run("overview_threads", func(test *testing.T) {
		harness := threadHarness(test, 100, 40)
		harness.press("l", "]", "]")
		golden.Assert(test, "overview_threads", harness.render(), false)
	})
	test.Run("diff_threads", func(test *testing.T) {
		harness := threadHarness(test, 100, 40)
		harness.selectFile()
		golden.Assert(test, "diff_threads", harness.render(), false)
	})
	test.Run("diff_threads_narrow", func(test *testing.T) {
		harness := threadHarness(test, 60, 24)
		harness.selectFile()
		golden.Assert(test, "diff_threads_narrow", harness.render(), false)
	})
}

func overviewThreadsText(harness *harness) string {
	harness.press("l", "G")
	return ansi.Strip(harness.render())
}

func TestOverviewThreadsLoadingState(test *testing.T) {
	harness := newHarness(test, 100, 40)
	for _, message := range messages(harness.view.Init()) {
		if _, ok := message.(threadsLoadedMessage); !ok { // the thread load is still in flight
			harness.send(message)
		}
	}
	out := overviewThreadsText(harness)
	if !strings.Contains(out, "Loading review threads…") || strings.Contains(out, "No review threads.") {
		test.Errorf("expected the loading state:\n%s", out)
	}
}

func TestOverviewThreadsErrorState(test *testing.T) {
	harness := newHarness(test, 100, 40)
	harness.client.SetError(fake.MethodListReviewThreads, errors.New("boom\x1b[2J"))
	harness.run(harness.view.Init())
	out := overviewThreadsText(harness)
	if !strings.Contains(out, "Could not load review threads: ") || !strings.Contains(out, "boom") || strings.Contains(out, "No review threads.") {
		test.Errorf("expected the error state:\n%s", out)
	}
	assertTame(test, "thread error state", harness.render())
	// The next load replaces it.
	harness.client.SetError(fake.MethodListReviewThreads, nil)
	harness.press("R")
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "No review threads.") || strings.Contains(out, "Could not load") {
		test.Errorf("expected the empty state after a good load:\n%s", out)
	}
}

func TestOverviewThreadsEmptyState(test *testing.T) {
	harness := loaded(test, 100, 40)
	if out := overviewThreadsText(harness); !strings.Contains(out, "Review threads (0)") || !strings.Contains(out, "No review threads.") {
		test.Errorf("expected the empty state:\n%s", out)
	}
}
