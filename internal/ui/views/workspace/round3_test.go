package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// messages runs cmd (and the commands of any batch) and returns what they produce.
func messages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch m := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range m {
			out = append(out, messages(c)...)
		}
		return out
	default:
		return []tea.Msg{m}
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
	for _, m := range newResults {
		second.Update(m)
	}
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

func TestRefreshKeepsFilesListTop(t *testing.T) {
	h := loaded(t, 100, 7) // three file rows visible
	h.press("G", "k", "k")
	top, cur := h.v.fw.Top(), h.v.sel()
	if top != 3 || cur != 3 {
		t.Fatalf("setup: top=%d cursor=%d, want 3 and 3", top, cur)
	}
	h.press("R")
	if h.v.fw.Top() != top || h.v.sel() != cur {
		t.Errorf("refresh moved the list: top=%d cursor=%d, want %d and %d", h.v.fw.Top(), h.v.sel(), top, cur)
	}
}

func TestRefreshWithShorterListStaysInBounds(t *testing.T) {
	h := loaded(t, 100, 7)
	h.press("G")
	d := testDetail()
	d.Files = d.Files[:2]
	h.send(prLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: h.v.seq, detail: d})
	if h.v.sel() < 0 || h.v.sel() > 2 {
		t.Errorf("selection = %d after the list shrank", h.v.sel())
	}
	_ = h.render()
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

	h.press("] f") // client.go parsed fine
	if out := ansi.Strip(h.render()); !strings.Contains(out, "func (c *Client) Sync") {
		t.Errorf("a file that parsed should show its diff:\n%s", out)
	}
	h.press("] f") // retry.go is the malformed one: what parsed before the problem shows
	if out := ansi.Strip(h.render()); !strings.Contains(out, "+one") {
		t.Errorf("the partly parsed file should show what parsed:\n%s", out)
	}
	h.press("] f") // legacy.go is not in the diff at all
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

func TestLoadingLineCleansRepo(t *testing.T) {
	v := New(fake.New(), github.Repo{Owner: "o\x1b[2J\x1b]52;c;aGk=\x07", Name: "n\u202e"}, 7, WithTheme(theme.New(true)))
	v.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	assertTame(t, "loading line", v.Render(80, 10))
}
