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
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestLoad(test *testing.T) {
	harness := newHarness(test, 100, 30)
	if got := harness.view.Title(); got != "#7" {
		test.Errorf("Title = %q", got)
	}
	command := harness.view.Init()
	if !harness.view.Loading() {
		test.Error("should be loading after Init")
	}
	harness.run(command)
	if harness.view.Loading() {
		test.Error("should have stopped loading")
	}
	if harness.view.detail == nil || harness.view.diff.Raw == "" {
		test.Fatalf("detail=%v diff=%q", harness.view.detail, harness.view.diff.Raw)
	}
	var calls []string
	for _, call := range harness.client.Calls() {
		calls = append(calls, call.Method)
	}
	if diff := cmp.Diff([]string{fake.MethodGetPullRequest, fake.MethodGetDiff, fake.MethodListReviewThreads}, calls); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestLoadErrors(test *testing.T) {
	tests := []struct {
		name     string
		err      error
		notFound bool
	}{
		{"not found", fmt.Errorf("load: %w", github.ErrNotFound), true},
		{"other", errors.New("boom"), false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := newHarness(test, 100, 30)
			harness.client.SetError(fake.MethodGetPullRequest, testCase.err)
			harness.run(harness.view.Init())
			if harness.view.notFound != testCase.notFound || harness.view.err == nil {
				test.Errorf("notFound=%v err=%v", harness.view.notFound, harness.view.err)
			}
			if !errors.Is(harness.lastErr(), testCase.err) {
				test.Errorf("status bar error = %v, want %v", harness.lastErr(), testCase.err)
			}
			if harness.view.Loading() == false && harness.view.diffPending {
				test.Error("inconsistent loading state")
			}
		})
	}
}

func TestRefreshKeepsContentOnError(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j", "j") // select the second file
	path := harness.view.selectedPath()
	harness.client.SetError(fake.MethodGetPullRequest, errors.New("flaky"))
	harness.press("R")
	if harness.view.detail == nil || harness.view.err != nil {
		test.Error("refresh failure must keep the loaded content")
	}
	if harness.lastErr() == nil {
		test.Error("refresh failure should reach the status bar")
	}
	harness.client.SetError(fake.MethodGetPullRequest, nil)
	harness.press("R")
	if harness.view.selectedPath() != path {
		test.Errorf("selection after refresh = %q, want %q", harness.view.selectedPath(), path)
	}
	count := 0
	for _, call := range harness.client.Calls() {
		if call.Method == fake.MethodGetPullRequest {
			count++
		}
	}
	if count != 3 {
		test.Errorf("GetPullRequest calls = %d, want 3", count)
	}
}

func TestStaleResultsIgnored(test *testing.T) {
	harness := loaded(test, 100, 30)
	other := testDetail()
	other.Title = "stale"
	current := harness.view.sequence
	for name, message := range map[string]pullRequestLoadedMessage{
		"old seq":      {owner: harness.view, repository: testRepository, number: 7, sequence: current - 1, detail: other},
		"other number": {owner: harness.view, repository: testRepository, number: 8, sequence: current, detail: other},
		"other repo":   {owner: harness.view, repository: github.Repository{Owner: "x", Name: "y"}, number: 7, sequence: current, detail: other},
	} {
		harness.send(message)
		if harness.view.detail.Title == "stale" {
			test.Errorf("%s: stale result applied", name)
		}
	}
	harness.send(diffLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: current - 1, raw: "stale"})
	if harness.view.diff.Raw == "stale" {
		test.Error("stale diff applied")
	}
	harness.press("R")
	previous := harness.view.sequence - 1
	harness.send(pullRequestLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: previous, detail: other})
	if harness.view.detail.Title == "stale" {
		test.Error("result of a superseded request applied")
	}
}

func TestDiffRefreshFailureKeepsDiff(test *testing.T) {
	harness := loaded(test, 100, 30)
	before := harness.view.diff.Raw
	harness.client.SetError(fake.MethodGetDiff, errors.New("blip"))
	harness.press("R")
	if harness.view.diff.Raw != before || harness.view.diff.Loading {
		test.Errorf("diff after failed refresh = %+v, want Raw kept and not loading", harness.view.diff)
	}
	if harness.lastErr() == nil {
		test.Error("diff failure should reach the status bar")
	}
	// A diff that became too large, with no file list to build it from
	// either, is a failed refresh too.
	harness.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
	harness.client.SetError(fake.MethodListFilePatches, errors.New("blip"))
	harness.press("R")
	if harness.view.diff.Raw != before || harness.view.diff.Err != nil {
		test.Errorf("diff after a too-large refresh with no file list = %+v, want Raw kept", harness.view.diff)
	}
}

// TestReopenedWorkspaceIgnoresOldResults: open #N, close it before it loads,
// open #N again. Both instances count requests from 1, so only the view
// identity tells their results apart.
func TestReopenedWorkspaceIgnoresOldResults(test *testing.T) {
	client := fake.New()
	old := testDetail()
	old.Title = "old snapshot"
	client.SetPullRequest(old)
	client.SetDiff(testRepository, 7, readDiff(test))
	options := []Option{WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true))}

	first := New(client, testRepository, 7, options...)
	oldResults := messages(first.Init()) // still "in flight" when it is closed

	fresh := testDetail()
	fresh.Title = "fresh data"
	client.SetPullRequest(fresh)
	second := New(client, testRepository, 7, options...)
	newResults := messages(second.Init())

	for _, message := range oldResults {
		second.Update(message)
	}
	if second.detail != nil || !second.Loading() {
		test.Fatalf("results of the closed workspace were accepted: detail=%v loading=%v", second.detail != nil, second.Loading())
	}
	feed(second, newResults)
	if second.detail == nil || second.detail.Title != "fresh data" || second.Loading() {
		test.Errorf("own results not applied: detail=%v loading=%v", second.detail, second.Loading())
	}
	// And the other way round: the new workspace's results mean nothing to the old one.
	for _, message := range newResults {
		first.Update(message)
	}
	if first.detail != nil {
		test.Error("the closed workspace accepted a newer instance's result")
	}
}

// malformedDiff has a good first file and a second whose hunk runs past its
// line counts.
func malformedDiff(test *testing.T) string {
	test.Helper()
	good, _, _ := strings.Cut(readDiff(test), "diff --git a/internal/sync/retry.go")
	return good + "diff --git a/internal/sync/retry.go b/internal/sync/retry.go\n" +
		"new file mode 100644\n--- /dev/null\n+++ b/internal/sync/retry.go\n" +
		"@@ -0,0 +1,1 @@\n+one\n+two\n+three\n"
}

func TestPartialDiffStillShowsParsedFiles(test *testing.T) {
	harness := newHarness(test, 100, 24)
	harness.client.SetDiff(testRepository, 7, malformedDiff(test))
	harness.run(harness.view.Init())

	harness.press("}") // client.go parsed fine
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "func (c *Client) Sync") {
		test.Errorf("a file that parsed should show its diff:\n%s", out)
	}
	harness.press("}") // retry.go is the malformed one: what parsed before the problem shows
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "+one") {
		test.Errorf("the partly parsed file should show what parsed:\n%s", out)
	}
	harness.press("}") // legacy.go is not in the diff at all
	out := ansi.Strip(harness.render())
	if !strings.Contains(out, "No diff available for this file.") || !strings.Contains(out, "could not be fully parsed") {
		test.Errorf("the missing file should say so:\n%s", out)
	}

	var errorTexts []string
	for _, message := range harness.messages {
		if errorMessage, ok := message.(userinterface.ErrorMessage); ok {
			errorTexts = append(errorTexts, errorMessage.Err.Error())
		}
	}
	if len(errorTexts) != 1 {
		test.Fatalf("status bar errors = %q, want exactly one", errorTexts)
	}
	if count := strings.Count(errorTexts[0], "parse diff"); count != 1 || !strings.HasPrefix(errorTexts[0], "acme/widgets#7: ") {
		test.Errorf("status bar text = %q", errorTexts[0])
	}
}
