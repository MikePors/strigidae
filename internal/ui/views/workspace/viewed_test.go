package workspace

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// viewedHarness loads the test pull request with the given viewed states
// applied to its files in order.
func viewedHarness(t *testing.T, w, h int, states ...github.ViewedState) *harness {
	t.Helper()
	hs := newHarness(t, w, h)
	d := testDetail()
	for i, s := range states {
		d.Files[i].Viewed = s
	}
	hs.client.SetPullRequest(d)
	hs.run(hs.v.Init())
	return hs
}

func (h *harness) stateOf() github.ViewedState {
	h.t.Helper()
	for _, f := range h.v.files() {
		if f.Path == firstFile {
			return f.Viewed
		}
	}
	h.t.Fatalf("no file %q", firstFile)
	return ""
}

func (h *harness) serverState() github.ViewedState {
	h.t.Helper()
	d, err := h.client.GetPullRequest(h.t.Context(), testRepo, 7)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, f := range d.Files {
		if f.Path == firstFile {
			return f.Viewed
		}
	}
	return ""
}

const firstFile = "internal/sync/client.go"

func viewedCalls(c *fake.Client) []fake.Call {
	var out []fake.Call
	for _, call := range c.Calls() {
		if call.Method == fake.MethodSetFileViewed {
			out = append(out, call)
		}
	}
	return out
}

func TestToggleViewed(t *testing.T) {
	tests := []struct {
		name     string
		start    github.ViewedState
		wantView bool
		want     github.ViewedState
	}{
		{"unviewed becomes viewed", github.FileUnviewed, true, github.FileViewed},
		{"unknown becomes viewed", "", true, github.FileViewed},
		{"dismissed becomes viewed", github.FileDismissed, true, github.FileViewed},
		{"viewed becomes unviewed", github.FileViewed, false, github.FileUnviewed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := viewedHarness(t, 100, 24, tt.start)
			h.press("j", "v")
			if got := h.stateOf(); got != tt.want {
				t.Errorf("state = %q, want %q", got, tt.want)
			}
			calls := viewedCalls(h.client)
			if len(calls) != 1 || calls[0].PullRequestID != "PR_7" || calls[0].Path != firstFile || calls[0].Viewed != tt.wantView {
				t.Errorf("calls = %+v", calls)
			}
			if got := h.serverState(); got != tt.want {
				t.Errorf("server state = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToggleViewedFromRightPane(t *testing.T) {
	h := viewedHarness(t, 100, 24)
	h.press("j", "l", "v")
	if got := h.stateOf(); got != github.FileViewed {
		t.Errorf("state = %q", got)
	}
}

func TestToggleViewedOnOverviewDoesNothing(t *testing.T) {
	h := viewedHarness(t, 100, 24)
	h.press("v")
	if n := len(viewedCalls(h.client)); n != 0 {
		t.Errorf("%d calls", n)
	}
	if h.viewedCountForTest() != 0 {
		t.Error("something was marked")
	}
}

func (h *harness) viewedCountForTest() int { return h.v.viewedCount() }

func TestToggleViewedRevertsOnError(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileDismissed)
	h.client.SetError(fake.MethodSetFileViewed, errors.New("boom \x1b[31mred"))
	h.press("j", "v")
	if got := h.stateOf(); got != github.FileDismissed {
		t.Errorf("state = %q, want the original dismissed", got)
	}
	err := h.lastErr()
	if err == nil || !strings.Contains(err.Error(), "boom") || strings.Contains(err.Error(), "\x1b") {
		t.Errorf("error = %v", err)
	}
}

func TestFilesTitleShowsProgress(t *testing.T) {
	h := viewedHarness(t, 100, 24, github.FileViewed, github.FileDismissed, github.FileViewed)
	if !strings.Contains(ansi.Strip(h.render()), "Files 2/5 viewed") {
		t.Errorf("no progress in title:\n%s", ansi.Strip(h.render()))
	}
	h.press("j", "j", "v") // second file: dismissed becomes viewed
	if !strings.Contains(ansi.Strip(h.render()), "Files 3/5 viewed") {
		t.Error("title did not update at once")
	}
}

func TestViewedRowsAreStyled(t *testing.T) {
	th := theme.New(true)
	// Wide enough that no path is truncated.
	h := viewedHarness(t, 140, 24, github.FileUnviewed, github.FileDismissed, github.FileUnviewed, github.FileUnviewed, github.FileViewed)
	out := h.render()
	open := func(s string) string { return strings.TrimSuffix(s, "\x1b[m") }
	if !strings.Contains(out, th.FileAdded.Render("✓")) {
		t.Error("viewed check is not styled with FileAdded")
	}
	if !strings.Contains(out, th.FileModified.Render("~")) {
		t.Error("dismissed mark is not styled with FileModified")
	}
	if !strings.Contains(out, open(th.Muted.Render("README.md"))) {
		t.Error("viewed path is not muted")
	}
	if !strings.Contains(out, open(th.Normal.Render("internal/sync/retry.go"))) {
		t.Error("dismissed path should stay normal")
	}
}

func TestGoldenViewedStates(t *testing.T) {
	h := viewedHarness(t, 100, 20, github.FileViewed, github.FileDismissed, github.FileUnviewed, github.FileViewed)
	h.press("j", "j")
	golden.Assert(t, "files_viewed", h.render(), false)
}
