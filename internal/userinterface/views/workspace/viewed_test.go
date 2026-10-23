package workspace

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// viewedHarness loads the test pull request with the given viewed states
// applied to its files in order.
func viewedHarness(test *testing.T, width, height int, states ...github.ViewedState) *harness {
	test.Helper()
	otherHarness := newHarness(test, width, height)
	detail := testDetail()
	for i, state := range states {
		detail.Files[i].Viewed = state
	}
	otherHarness.client.SetPullRequest(detail)
	otherHarness.run(otherHarness.v.Init())
	return otherHarness
}

func (harness *harness) stateOf() github.ViewedState {
	harness.t.Helper()
	for _, file := range harness.v.files() {
		if file.Path == firstFile {
			return file.Viewed
		}
	}
	harness.t.Fatalf("no file %q", firstFile)
	return ""
}

func (harness *harness) serverState() github.ViewedState {
	harness.t.Helper()
	detail, err := harness.client.GetPullRequest(harness.t.Context(), testRepository, 7)
	if err != nil {
		harness.t.Fatal(err)
	}
	for _, file := range detail.Files {
		if file.Path == firstFile {
			return file.Viewed
		}
	}
	return ""
}

const firstFile = "internal/sync/client.go"

func viewedCalls(client *fake.Client) []fake.Call {
	var out []fake.Call
	for _, call := range client.Calls() {
		if call.Method == fake.MethodSetFileViewed {
			out = append(out, call)
		}
	}
	return out
}

func TestToggleViewed(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := viewedHarness(test, 100, 24, testCase.start)
			harness.press("j", "v")
			if got := harness.stateOf(); got != testCase.want {
				test.Errorf("state = %q, want %q", got, testCase.want)
			}
			calls := viewedCalls(harness.client)
			if len(calls) != 1 || calls[0].PullRequestIdentifier != "PR_7" || calls[0].Path != firstFile || calls[0].Viewed != testCase.wantView {
				test.Errorf("calls = %+v", calls)
			}
			if got := harness.serverState(); got != testCase.want {
				test.Errorf("server state = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestToggleViewedFromRightPane(test *testing.T) {
	harness := viewedHarness(test, 100, 24)
	harness.press("j", "l", "v")
	if got := harness.stateOf(); got != github.FileViewed {
		test.Errorf("state = %q", got)
	}
}

func TestToggleViewedOnOverviewDoesNothing(test *testing.T) {
	harness := viewedHarness(test, 100, 24)
	harness.press("v")
	if count := len(viewedCalls(harness.client)); count != 0 {
		test.Errorf("%d calls", count)
	}
	if harness.viewedCountForTest() != 0 {
		test.Error("something was marked")
	}
}

func (harness *harness) viewedCountForTest() int { return harness.v.viewedCount() }

func TestToggleViewedRevertsOnError(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileDismissed)
	harness.client.SetError(fake.MethodSetFileViewed, errors.New("boom \x1b[31mred"))
	harness.press("j", "v")
	if got := harness.stateOf(); got != github.FileDismissed {
		test.Errorf("state = %q, want the original dismissed", got)
	}
	err := harness.lastErr()
	if err == nil || !strings.Contains(err.Error(), "boom") || strings.Contains(err.Error(), "\x1b") {
		test.Errorf("error = %v", err)
	}
}

func TestFilesTitleShowsProgress(test *testing.T) {
	harness := viewedHarness(test, 100, 24, github.FileViewed, github.FileDismissed, github.FileViewed)
	if !strings.Contains(ansi.Strip(harness.render()), "Files 2/5 viewed") {
		test.Errorf("no progress in title:\n%s", ansi.Strip(harness.render()))
	}
	harness.press("j", "j", "v") // second file: dismissed becomes viewed
	if !strings.Contains(ansi.Strip(harness.render()), "Files 3/5 viewed") {
		test.Error("title did not update at once")
	}
}

func TestViewedRowsAreStyled(test *testing.T) {
	activeTheme := theme.New(true)
	// Wide enough that no path is truncated.
	harness := viewedHarness(test, 140, 24, github.FileUnviewed, github.FileDismissed, github.FileUnviewed, github.FileUnviewed, github.FileViewed)
	out := harness.render()
	open := func(text string) string { return strings.TrimSuffix(text, "\x1b[m") }
	if !strings.Contains(out, activeTheme.FileAdded.Render("✓")) {
		test.Error("viewed check is not styled with FileAdded")
	}
	if !strings.Contains(out, activeTheme.FileModified.Render("↻")) {
		test.Error("dismissed mark is not styled with FileModified")
	}
	if !strings.Contains(out, open(activeTheme.Muted.Render("README.md"))) {
		test.Error("viewed path is not muted")
	}
	if !strings.Contains(out, open(activeTheme.Normal.Render("internal/sync/retry.go"))) {
		test.Error("dismissed path should stay normal")
	}
}

func TestGoldenViewedStates(test *testing.T) {
	harness := viewedHarness(test, 100, 20, github.FileViewed, github.FileDismissed, github.FileUnviewed, github.FileViewed)
	harness.press("j", "j")
	golden.Assert(test, "files_viewed", harness.render(), false)
}
