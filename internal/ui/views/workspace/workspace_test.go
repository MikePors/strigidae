package workspace

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

var (
	testNow  = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	testRepo = github.Repo{Owner: "acme", Name: "widgets"}
)

const longBody = `## Summary

This change adds a **retry policy** to the sync client so transient network
failures no longer abort a whole run. It replaces the ad-hoc loop in
` + "`sync.go`" + ` with a small, well-tested helper.

### What changed

- Added ` + "`Retry`" + ` with exponential backoff and jitter
- Moved the timeout handling into the helper
- Deleted the legacy ` + "`retryForever`" + ` function

### Example

` + "```go" + `
err := Retry(ctx, 5, func() error {
	return client.Sync(ctx)
})
` + "```" + `

### Notes

The backoff starts at 100ms and doubles up to a cap of 10s. See the design
doc for why jitter matters when many clients restart at once, and the
follow-up issue for making the policy configurable.

1. Review the helper first
2. Then the call sites
3. Finally the removed code`

func testDetail() github.PullRequestDetail {
	return github.PullRequestDetail{
		PullRequest: github.PullRequest{
			Repo: testRepo, Number: 7, Title: "Add retry policy to sync client",
			Author: "alice", State: github.PRStateOpen,
			ReviewDecision: github.ReviewDecisionChangesRequested,
			HeadRef:        "feat/retry", BaseRef: "main",
			URL: "https://example.test/acme/widgets/pull/7",
		},
		Body:           longBody,
		Labels:         []github.Label{{Name: "enhancement"}, {Name: "sync"}},
		ReviewRequests: []github.Reviewer{{Name: "bob"}, {Name: "core", IsTeam: true}},
		Files: []github.ChangedFile{
			{Path: "internal/sync/client.go", Additions: 25, Change: github.FileModified, Deletions: 0},
			{Path: "internal/sync/retry.go", Additions: 214, Change: github.FileAdded},
			{Path: "internal/sync/legacy.go", Deletions: 90, Change: github.FileDeleted},
			{Path: "docs/design/a-very-long-directory-name/retry-policy-and-backoff.md", Additions: 12, Deletions: 3, Change: github.FileRenamed},
			{Path: "README.md", Additions: 2, Deletions: 1, Change: github.FileModified},
		},
		Comments: []github.IssueComment{
			{Author: "bob", CreatedAt: testNow.Add(-48 * time.Hour), Body: "Thanks! Could you add a test for the **jitter** bounds?"},
			{Author: "alice", CreatedAt: testNow.Add(-26 * time.Hour), Body: "Done in the latest push."},
		},
		Reviews: []github.Review{
			{Author: "carol", State: github.ReviewChangesRequested, SubmittedAt: testNow.Add(-30 * time.Hour), Body: "The cap should be configurable."},
			{Author: "dave", State: github.ReviewCommented, SubmittedAt: testNow.Add(-29 * time.Hour)},
			{Author: "me", State: github.ReviewPending, Body: "Draft: looks good overall, nit on naming."},
			{Author: "bob", State: github.ReviewApproved, SubmittedAt: testNow.Add(-90 * time.Minute)},
		},
	}
}

type harness struct {
	t      *testing.T
	v      *View
	client *fake.Client
	opened []string
	msgs   []tea.Msg // ui messages emitted by commands
}

func readDiff(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/pr.diff")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newHarness(t *testing.T, w, h int) *harness {
	t.Helper()
	c := fake.New()
	c.SetPullRequest(testDetail())
	c.SetDiff(testRepo, 7, readDiff(t))
	hs := &harness{t: t, client: c}
	hs.v = New(c, testRepo, 7,
		WithClock(func() time.Time { return testNow }),
		WithTheme(theme.New(true)),
		WithOpener(func(u string) error { hs.opened = append(hs.opened, u); return nil }),
	)
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	return hs
}

// loaded returns a harness whose Init results have been delivered.
func loaded(t *testing.T, w, h int) *harness {
	hs := newHarness(t, w, h)
	hs.run(hs.v.Init())
	return hs
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	v, cmd := h.v.Update(msg)
	h.v = v.(*View)
	h.run(cmd)
}

// run executes cmd, delivering result messages back to the view.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case ui.InfoMsg, ui.ErrorMsg:
		h.msgs = append(h.msgs, msg)
	default:
		h.send(msg)
	}
}

func (h *harness) press(seqs ...string) {
	h.t.Helper()
	for _, s := range seqs {
		h.send(keys.Msg{Seq: s})
	}
}

// typ sends raw key presses, as the root does while the view captures keys.
func (h *harness) typ(text string) {
	h.t.Helper()
	for _, r := range text {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func (h *harness) render() string { return h.v.Render(h.v.width, h.v.height) }

func (h *harness) lastErr() error {
	for i := len(h.msgs) - 1; i >= 0; i-- {
		if e, ok := h.msgs[i].(ui.ErrorMsg); ok {
			return e.Err
		}
	}
	return nil
}

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
	if diff := cmp.Diff([]string{fake.MethodGetPullRequest, fake.MethodGetDiff}, calls); diff != "" {
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

func TestFocusAndFileNavigation(t *testing.T) {
	h := loaded(t, 100, 30)
	if h.v.focus != paneFiles || h.v.sel() != 0 {
		t.Fatalf("initial focus=%v sel=%d", h.v.focus, h.v.sel())
	}
	h.press("l")
	if h.v.focus != paneRight {
		t.Error("l should focus the right pane")
	}
	h.press("h")
	if h.v.focus != paneFiles {
		t.Error("h should focus the files pane")
	}

	h.press("j", "j")
	if h.v.sel() != 2 {
		t.Errorf("j j: sel = %d", h.v.sel())
	}
	h.press("k")
	if h.v.sel() != 1 {
		t.Errorf("k: sel = %d", h.v.sel())
	}
	h.press("G")
	if h.v.sel() != 5 {
		t.Errorf("G: sel = %d", h.v.sel())
	}
	h.press("j")
	if h.v.sel() != 5 {
		t.Errorf("j past the end: sel = %d", h.v.sel())
	}
	h.press("g g")
	if h.v.sel() != 0 {
		t.Errorf("g g: sel = %d", h.v.sel())
	}
	h.press("ctrl+f")
	if h.v.sel() != 5 {
		t.Errorf("ctrl+f: sel = %d", h.v.sel())
	}

	// enter in the Files pane focuses the right pane.
	h.press("enter")
	if h.v.focus != paneRight {
		t.Error("enter should focus the right pane")
	}

	// ] f and [ f work from either pane and never land on Overview.
	h.press("g g") // scrolls the overview, selection unchanged
	if h.v.sel() != 5 {
		t.Errorf("g g in the right pane moved the selection to %d", h.v.sel())
	}
	h.press("[ f")
	if h.v.sel() != 4 {
		t.Errorf("[ f: sel = %d", h.v.sel())
	}
	h.press("h", "g g", "[ f")
	if h.v.sel() != 0 {
		t.Errorf("[ f from Overview should stay: sel = %d", h.v.sel())
	}
	h.press("] f")
	if h.v.sel() != 1 {
		t.Errorf("] f from Overview: sel = %d", h.v.sel())
	}
	h.press("l", "] f", "] f", "] f", "] f", "] f")
	if h.v.sel() != 5 {
		t.Errorf("] f clamps at the last file: sel = %d", h.v.sel())
	}
}

func TestFilePaneFollowsSelection(t *testing.T) {
	h := loaded(t, 100, 30)
	h.press("j")
	if !strings.Contains(ansi.Strip(h.render()), "func (c *Client) Sync") {
		t.Error("selecting a file should show its diff")
	}
	if got := h.v.rightTitle(); got != "internal/sync/client.go" {
		t.Errorf("right pane title = %q", got)
	}
	h.press("g g")
	if got := h.v.rightTitle(); got != "Overview" {
		t.Errorf("right pane title = %q", got)
	}
}

func TestSectionJumps(t *testing.T) {
	h := loaded(t, 100, 14)
	h.press("l")
	if h.v.ov.vp.YOffset() != 0 {
		t.Fatal("overview should start at the top")
	}
	h.press("] s")
	conv := h.v.ov.sections[1]
	if got := h.v.ov.vp.YOffset(); got != conv {
		t.Errorf("] s: offset = %d, want %d", got, conv)
	}
	h.press("[ s")
	if got := h.v.ov.vp.YOffset(); got != 0 {
		t.Errorf("[ s: offset = %d, want 0", got)
	}
	// Works from the Files pane as well.
	h.press("h", "] s")
	if got := h.v.ov.vp.YOffset(); got != conv {
		t.Errorf("] s from files: offset = %d, want %d", got, conv)
	}
	// Ignored while a file is selected.
	h.press("j", "[ s")
	h.press("g g", "] s")
}

func TestOverviewScrolling(t *testing.T) {
	h := loaded(t, 100, 14)
	h.press("l", "j")
	if h.v.ov.vp.YOffset() != 1 {
		t.Errorf("j: offset = %d", h.v.ov.vp.YOffset())
	}
	h.press("k")
	h.press("ctrl+d")
	if h.v.ov.vp.YOffset() == 0 {
		t.Error("ctrl+d should scroll")
	}
	h.press("G")
	if !h.v.ov.vp.AtBottom() {
		t.Error("G should reach the bottom")
	}
	h.press("g g")
	if !h.v.ov.vp.AtTop() {
		t.Error("g g should reach the top")
	}
	h.press("ctrl+f", "ctrl+b", "ctrl+u")
}

func TestConversationOrder(t *testing.T) {
	d := testDetail()
	got := conversation(&d)
	var authors []string
	for _, e := range got {
		authors = append(authors, e.author)
	}
	// carol -30h, bob comment -48h => bob, alice(-26h)... sorted by time, pending last.
	want := []string{"bob", "carol", "alice", "bob", "me"}
	if diff := cmp.Diff(want, authors); diff != "" {
		t.Errorf("conversation order (-want +got):\n%s", diff)
	}
	if !got[len(got)-1].pending {
		t.Error("pending review should be last and flagged")
	}
}

func TestRelTime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "just now"},
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{50 * time.Hour, "2d ago"},
		{75 * 24 * time.Hour, "2mo ago"},
		{800 * 24 * time.Hour, "2y ago"},
	}
	for _, tt := range tests {
		if got := relTime(testNow, testNow.Add(-tt.d)); got != tt.want {
			t.Errorf("relTime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFilePicker(t *testing.T) {
	h := loaded(t, 100, 24)
	if h.v.CapturesKeys() {
		t.Fatal("picker should start closed")
	}
	h.press("f")
	if !h.v.CapturesKeys() {
		t.Fatal("f should open the picker")
	}

	// Filtering narrows the list; enter jumps to the match.
	h.typ("retry")
	if got := h.v.picker.Matches(); got != 2 {
		t.Errorf("matches for %q = %d, want 2 (retry.go, retry-policy doc)", h.v.picker.Query(), got)
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.v.CapturesKeys() {
		t.Error("enter should close the picker")
	}
	if got := h.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		t.Errorf("jumped to %q", got)
	}

	// esc cancels and leaves the selection alone.
	h.press("f")
	h.typ("read")
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.v.CapturesKeys() {
		t.Error("esc should close the picker")
	}
	if got := h.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		t.Errorf("esc changed selection to %q", got)
	}

	// Typed letters are filter text, not commands; backspace edits.
	h.press("f")
	h.typ("jl")
	if h.v.focus != paneFiles {
		t.Error("typing in the picker leaked into the view")
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	h.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if h.v.picker.Matches() != 5 {
		t.Errorf("empty filter should list every file, got %d", h.v.picker.Matches())
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func TestFilePickerNoMatchEnter(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("f")
	h.typ("zzzz")
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !h.v.CapturesKeys() {
		t.Error("enter with no matches should keep the picker open")
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

// TestCapturerReleasesOnEsc is the workspace's entry for the esc-releases
// capture rule (see ui.TestCapturersReleaseOnEsc, which cannot import views),
// driven through the real root so key routing is covered too.
func TestCapturerReleasesOnEsc(t *testing.T) {
	h := loaded(t, 100, 24)
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	m := ui.New(h.v, theme.New(true), ui.Options{Tick: noTick})
	step := func(msg tea.Msg) {
		t.Helper()
		_, _ = m.Update(msg) // commands (timers, spinner) are not run
	}
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	for _, k := range []string{"]", "f"} {
		step(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	if h.v.sel() != 1 {
		t.Fatalf("] f through the root: sel = %d", h.v.sel())
	}
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !h.v.CapturesKeys() {
		t.Fatal("f through the root should open the picker")
	}
	step(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if h.v.picker.Query() != "q" {
		t.Errorf("key did not reach the picker raw: %q", h.v.picker.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.v.CapturesKeys() {
		t.Error("view still captures keys after esc")
	}
}

func TestOpenInBrowser(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("o")
	if diff := cmp.Diff([]string{"https://example.test/acme/widgets/pull/7"}, h.opened); diff != "" {
		t.Errorf("opened (-want +got):\n%s", diff)
	}

	h2 := newHarness(t, 100, 24) // not loaded yet
	h2.press("o")
	if len(h2.opened) != 0 || len(h2.msgs) == 0 {
		t.Errorf("o before load: opened=%v msgs=%v", h2.opened, h2.msgs)
	}

	h3 := loaded(t, 100, 24)
	h3.v.open = func(string) error { return errors.New("no browser") }
	h3.press("o")
	if h3.lastErr() == nil {
		t.Error("browser failure should reach the status bar")
	}
}

func TestThemeMessageRebuilds(t *testing.T) {
	h := loaded(t, 100, 24)
	dark := h.render()
	h.send(ui.ThemeMsg{Theme: theme.New(false)})
	if h.render() == dark {
		t.Error("light theme should change the rendering")
	}
}

func TestRendersWithoutPanicAtTinySizes(t *testing.T) {
	for _, sz := range [][2]int{{0, 0}, {3, 3}, {10, 4}, {20, 6}, {79, 5}, {80, 5}} {
		h := loaded(t, sz[0], sz[1])
		h.press("f", "j")
		_ = h.render()
		h.press("l")
		_ = h.render()
	}
}

// fixtures for the golden tests.

func TestGolden(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		h := loaded(t, 100, 40)
		golden.Assert(t, "overview", h.render(), false)
	})
	t.Run("overview_scrolled_to_conversation", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("l", "] s")
		golden.Assert(t, "overview_conversation", h.render(), false)
	})
	t.Run("file_selected", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("] f", "] f", "l")
		golden.Assert(t, "file_selected", h.render(), false)
	})
	t.Run("right_focused", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("l")
		golden.Assert(t, "right_focused", h.render(), true)
	})
	t.Run("picker", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("f")
		h.typ("retry")
		golden.Assert(t, "picker", h.render(), false)
	})
	t.Run("narrow_files", func(t *testing.T) {
		h := loaded(t, 60, 20)
		h.press("j")
		golden.Assert(t, "narrow_files", h.render(), false)
	})
	t.Run("narrow_right", func(t *testing.T) {
		h := loaded(t, 60, 20)
		h.press("enter")
		golden.Assert(t, "narrow_right", h.render(), false)
	})
	t.Run("loading", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		golden.Assert(t, "loading", h.render(), false)
	})
	t.Run("error", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		h.client.SetError(fake.MethodGetPullRequest, errors.New("load pull request: connection refused"))
		h.run(h.v.Init())
		golden.Assert(t, "error", h.render(), false)
	})
	t.Run("not_found", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		h.v.number = 99
		h.run(h.v.Init())
		golden.Assert(t, "not_found", h.render(), false)
	})
	t.Run("no_files_no_description", func(t *testing.T) {
		d := testDetail()
		d.Body, d.Files, d.Comments, d.Reviews = "", nil, nil, nil
		c := fake.New()
		c.SetPullRequest(d)
		c.SetDiff(testRepo, 7, readDiff(t))
		v := New(c, testRepo, 7, WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true)))
		h := &harness{t: t, v: v, client: c}
		h.send(tea.WindowSizeMsg{Width: 80, Height: 12})
		h.run(v.Init())
		h.press("] f", "f")
		golden.Assert(t, "empty", h.render(), false)
	})
}
