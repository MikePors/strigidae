package prlist

import (
	"errors"
	"fmt"
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
	testRepo = github.Repo{Owner: "acme", Name: "widgets"}
	testNow  = time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
)

func makePRs(n int) []github.PullRequest {
	prs := make([]github.PullRequest, n)
	for i := range prs {
		prs[i] = github.PullRequest{
			Repo:      testRepo,
			Number:    n - i,
			Title:     fmt.Sprintf("Change number %d", n-i),
			Author:    "alice",
			State:     github.PRStateOpen,
			URL:       fmt.Sprintf("https://github.com/acme/widgets/pull/%d", n-i),
			UpdatedAt: testNow.Add(-time.Duration(i) * time.Hour),
		}
	}
	return prs
}

func newView(client github.Client, open func(string) error) *View {
	v := New(client, testRepo, theme.Default(), Options{Now: func() time.Time { return testNow }, Open: open})
	v.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	return v
}

// deliver runs cmd and feeds the resulting message back into the view.
func deliver(t *testing.T, v *View, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	_, next := v.Update(cmd())
	return next
}

func press(v *View, seq string) tea.Cmd {
	_, cmd := v.Update(keys.Msg{Seq: seq})
	return cmd
}

func loadedView(t *testing.T, client github.Client, open func(string) error) *View {
	t.Helper()
	v := newView(client, open)
	deliver(t, v, v.Init())
	return v
}

func fakeWith(prs []github.PullRequest) *fake.Client {
	c := fake.New()
	c.SetPullRequests(testRepo, prs)
	return c
}

func TestFirstLoad(t *testing.T) {
	c := fakeWith(makePRs(3))
	v := newView(c, nil)
	if !v.Loading() {
		t.Fatal("new view should be loading")
	}
	if v.Title() != "acme/widgets" {
		t.Errorf("title = %q", v.Title())
	}
	if cmd := deliver(t, v, v.Init()); cmd != nil {
		t.Errorf("single page should not trigger more loading")
	}
	if v.Loading() || len(v.prs) != 3 {
		t.Errorf("loading=%v prs=%d, want loaded with 3", v.Loading(), len(v.prs))
	}
	want := []fake.Call{{Method: fake.MethodListPullRequests, Repo: testRepo}}
	if diff := cmp.Diff(want, c.Calls()); diff != "" {
		t.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestPaginationTrigger(t *testing.T) {
	c := fakeWith(makePRs(45))
	v := loadedView(t, c, nil)
	if len(v.prs) != 30 || v.next == "" {
		t.Fatalf("first page: prs=%d next=%q", len(v.prs), v.next)
	}

	// Far from the end: no request.
	if cmd := press(v, "j"); cmd != nil {
		t.Fatal("moving near the top should not load more")
	}
	// Jump to the end: the next page is requested once.
	cmd := press(v, "G")
	if cmd == nil || !v.Loading() {
		t.Fatal("reaching the end should load the next page")
	}
	if again := press(v, "k"); again != nil {
		t.Error("a load is in flight; no second request")
	}
	deliver(t, v, cmd)
	if len(v.prs) != 45 || v.next != "" {
		t.Errorf("after page 2: prs=%d next=%q", len(v.prs), v.next)
	}
	calls := c.Calls()
	if len(calls) != 2 || calls[1].Page.After != "30" {
		t.Errorf("calls = %+v, want second call After=30", calls)
	}
	if cmd := press(v, "G"); cmd != nil {
		t.Error("no more pages; no request")
	}
}

func TestStaleResultsIgnored(t *testing.T) {
	c := fakeWith(makePRs(3))
	v := newView(c, nil)
	first := v.Init()

	refresh := press(v, "R")
	if refresh == nil {
		t.Fatal("R should reload")
	}
	// The first load finishes after the refresh was requested.
	deliver(t, v, first)
	if len(v.prs) != 0 || !v.Loading() {
		t.Errorf("stale result applied: prs=%d loading=%v", len(v.prs), v.Loading())
	}
	// A result for another repository is not ours either.
	v.Update(prsLoadedMsg{repo: github.Repo{Owner: "x", Name: "y"}, seq: v.seq, page: github.Page[github.PullRequest]{Items: makePRs(1)}})
	if len(v.prs) != 0 {
		t.Error("other repo's result applied")
	}
	deliver(t, v, refresh)
	if len(v.prs) != 3 || v.Loading() {
		t.Errorf("refresh result not applied: prs=%d", len(v.prs))
	}
}

func TestRefreshKeepsSelection(t *testing.T) {
	c := fakeWith(makePRs(10))
	v := loadedView(t, c, nil)
	press(v, "j")
	press(v, "j")
	want := v.prs[v.cursor].Number
	deliver(t, v, press(v, "R"))
	if got := v.prs[v.cursor].Number; got != want {
		t.Errorf("selected #%d after refresh, want #%d", got, want)
	}
}

func TestRefreshKeepsSelectionPastFirstPage(t *testing.T) {
	c := fakeWith(makePRs(45))
	v := loadedView(t, c, nil)
	deliver(t, v, press(v, "G"))
	press(v, "G")
	if len(v.prs) != 45 || v.cursor != 44 {
		t.Fatalf("setup: prs=%d cursor=%d", len(v.prs), v.cursor)
	}
	want := v.prs[v.cursor].Number

	deliver(t, v, press(v, "R"))
	calls := c.Calls()
	if got := calls[len(calls)-1].Page.Limit; got != 45 {
		t.Errorf("refresh limit = %d, want 45", got)
	}
	if len(v.prs) != 45 || v.prs[v.cursor].Number != want {
		t.Errorf("after refresh: prs=%d selected #%d, want 45 rows and #%d", len(v.prs), v.prs[v.cursor].Number, want)
	}
}

func TestRefreshSelectionGone(t *testing.T) {
	c := fakeWith(makePRs(10))
	v := loadedView(t, c, nil)
	for range 4 {
		press(v, "j")
	}
	// The selected PR disappears from the list; the row index is kept.
	c.SetPullRequests(testRepo, makePRs(10)[:3])
	press(v, "R")
	v.Update(prsLoadedMsg{repo: testRepo, seq: v.seq, page: github.Page[github.PullRequest]{Items: makePRs(3)}})
	if v.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped)", v.cursor)
	}
}

func TestLoadError(t *testing.T) {
	c := fakeWith(makePRs(3))
	boom := errors.New("boom")
	c.SetError(fake.MethodListPullRequests, boom)
	v := newView(c, nil)
	cmd := deliver(t, v, v.Init())
	if v.Loading() {
		t.Error("still loading after error")
	}
	msg, ok := cmd().(ui.ErrorMsg)
	if !ok || !errors.Is(msg.Err, boom) {
		t.Fatalf("cmd produced %#v, want ui.ErrorMsg wrapping boom", msg)
	}
	if !strings.Contains(v.Render(80, 10), "boom") {
		t.Error("error state should show the error")
	}
}

func TestFailedNextPageIsNotRetried(t *testing.T) {
	c := fakeWith(makePRs(45))
	v := loadedView(t, c, nil)
	cmd := press(v, "G")
	c.SetError(fake.MethodListPullRequests, errors.New("rate limited"))
	errCmd := deliver(t, v, cmd)
	if _, ok := errCmd().(ui.ErrorMsg); !ok {
		t.Fatal("failed page should report an error")
	}
	if v.Loading() {
		t.Error("still loading after failure")
	}
	calls := len(c.Calls())
	for _, k := range []string{"k", "j", "G", "j"} {
		if cmd := press(v, k); cmd != nil {
			t.Fatalf("key %q retried the failed page", k)
		}
	}
	if len(c.Calls()) != calls {
		t.Error("extra requests were made")
	}

	// R clears the flag; once it succeeds, paging resumes.
	c.SetError(fake.MethodListPullRequests, nil)
	// The selection is still at the end, so the refresh result itself pages on.
	if more := deliver(t, v, press(v, "R")); more == nil {
		t.Error("paging should resume after a successful refresh")
	}
}

func TestOpenInBrowser(t *testing.T) {
	var opened []string
	open := func(u string) error { opened = append(opened, u); return nil }
	v := loadedView(t, fakeWith(makePRs(3)), open)

	cmd := press(v, "o")
	if cmd == nil {
		t.Fatal("o should produce a command")
	}
	if len(opened) != 0 {
		t.Fatal("browser launched during Update; it must run in the command")
	}
	msg, ok := cmd().(ui.InfoMsg)
	if !ok || len(opened) != 1 || opened[0] != "https://github.com/acme/widgets/pull/3" {
		t.Errorf("opened=%v msg=%#v", opened, msg)
	}

	failing := loadedView(t, fakeWith(makePRs(1)), func(string) error { return errors.New("no browser") })
	if _, ok := press(failing, "o")().(ui.ErrorMsg); !ok {
		t.Error("launch failure should become ui.ErrorMsg")
	}

	empty := loadedView(t, fakeWith(nil), open)
	if cmd := press(empty, "o"); cmd != nil {
		t.Error("o on an empty list should do nothing")
	}
}

func TestEnterShowsInfo(t *testing.T) {
	v := loadedView(t, fakeWith(makePRs(2)), nil)
	cmd := press(v, "enter")
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}
	if diff := cmp.Diff(ui.InfoMsg{Text: "PR workspace arrives with #7"}, cmd()); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestNavigation(t *testing.T) {
	v := loadedView(t, fakeWith(makePRs(25)), nil) // window shows 11 rows
	tests := []struct {
		keys []string
		want int
	}{
		{[]string{"j", "j", "j"}, 3},
		{[]string{"j", "k", "k"}, 0},
		{[]string{"G"}, 24},
		{[]string{"G", "g g"}, 0},
		{[]string{"ctrl+d"}, 5},
		{[]string{"ctrl+f"}, 11},
		{[]string{"ctrl+f", "ctrl+f", "ctrl+f"}, 24},
		{[]string{"ctrl+f", "ctrl+b"}, 0},
		{[]string{"G", "ctrl+u"}, 19},
	}
	for _, tt := range tests {
		press(v, "g g")
		for _, k := range tt.keys {
			press(v, k)
		}
		if v.cursor != tt.want {
			t.Errorf("keys %v: cursor = %d, want %d", tt.keys, v.cursor, tt.want)
		}
	}
}

func TestHostileTextIsNeutralised(t *testing.T) {
	prs := makePRs(1)
	prs[0].Title = "evil\x1b]52;c;aGk=\x07\x1b[2J title\u202e\u009b"
	prs[0].Author = "a\x1b[31muthor"
	v := loadedView(t, fakeWith(prs), nil)
	out := ansi.Strip(v.Render(80, 6))
	for _, r := range out {
		if r != '\n' && (r < ' ' || (r >= 0x7f && r <= 0x9f) || r == '\u202e') {
			t.Fatalf("control rune %q reached the screen: %q", r, out)
		}
	}
}

func TestRelativeAge(t *testing.T) {
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{-time.Hour, "now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{49 * time.Hour, "2d"},
		{45 * 24 * time.Hour, "1mo"},
		{400 * 24 * time.Hour, "1y"},
	}
	for _, tt := range tests {
		if got := relativeAge(testNow, testNow.Add(-tt.ago)); got != tt.want {
			t.Errorf("age %v = %q, want %q", tt.ago, got, tt.want)
		}
	}
}

func goldenPRs() []github.PullRequest {
	prs := makePRs(8)
	prs[0].Title = "Add a really quite extraordinarily long title that cannot possibly fit in the available columns"
	prs[0].Author = "someone-with-a-long-name"
	prs[0].ReviewDecision = github.ReviewDecisionApproved
	prs[0].CommentCount = 4
	prs[1].Draft = true
	prs[1].Title = "WIP: rework the parser"
	prs[1].CommentCount = 0
	prs[2].ReviewDecision = github.ReviewDecisionChangesRequested
	prs[2].CommentCount = 120
	prs[3].ReviewDecision = github.ReviewDecisionReviewRequired
	prs[3].Draft = true
	prs[3].Title = "Draft with a very long title that gets truncated after the marker is shown"
	prs[4].UpdatedAt = testNow.Add(-50 * time.Hour)
	prs[5].UpdatedAt = testNow.Add(-100 * 24 * time.Hour)
	return prs
}

func TestGolden(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) *View
		width, height int
	}{
		{"populated", func(t *testing.T) *View {
			v := loadedView(t, fakeWith(goldenPRs()), nil)
			press(v, "j")
			press(v, "j")
			return v
		}, 80, 12},
		{"scrolled", func(t *testing.T) *View {
			v := loadedView(t, fakeWith(makePRs(20)), nil)
			press(v, "G")
			return v
		}, 80, 8},
		{"narrow", func(t *testing.T) *View {
			return loadedView(t, fakeWith(goldenPRs()), nil)
		}, 36, 10},
		{"loading", func(*testing.T) *View { return newView(fakeWith(nil), nil) }, 80, 6},
		{"empty", func(t *testing.T) *View { return loadedView(t, fakeWith(nil), nil) }, 80, 6},
		{"error", func(t *testing.T) *View {
			c := fakeWith(nil)
			c.SetError(fake.MethodListPullRequests, fmt.Errorf("list pull requests: %w", github.ErrNotFound))
			v := newView(c, nil)
			deliver(t, v, v.Init())
			return v
		}, 40, 8},
		{"tiny", func(t *testing.T) *View {
			return loadedView(t, fakeWith(goldenPRs()), nil)
		}, 12, 3},
		{"error_tiny", func(t *testing.T) *View {
			c := fakeWith(nil)
			c.SetError(fake.MethodListPullRequests, fmt.Errorf("list pull requests: %w", github.ErrNotFound))
			v := newView(c, nil)
			deliver(t, v, v.Init())
			return v
		}, 20, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.setup(t)
			out := v.Render(tt.width, tt.height)
			lines := strings.Split(out, "\n")
			if len(lines) > tt.height {
				t.Errorf("%d lines, max %d", len(lines), tt.height)
			}
			for i, line := range lines {
				if w := ansi.StringWidth(line); w > tt.width {
					t.Errorf("line %d is %d columns wide, max %d: %q", i, w, tt.width, line)
				}
			}
			golden.Assert(t, tt.name, out, false)
		})
	}
}
