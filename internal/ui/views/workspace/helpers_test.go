package workspace

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
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
			ID:   "PR_7",
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
	case ui.InfoMsg, ui.ErrorMsg, ui.ConfirmMsg:
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
