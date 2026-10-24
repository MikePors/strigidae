package workspace

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func testChecks() []github.Check {
	return []github.Check{
		{Name: "CI / vulnerabilities", Result: github.CheckPassed, URL: "https://example.test/vulnerabilities"},
		{Name: "docs", Result: github.CheckSkipped},
		{Name: "CI / check", Result: github.CheckFailed, URL: "https://example.test/check"},
		{Name: "deploy", Result: github.CheckRunning, URL: "https://example.test/deploy"},
		{Name: "ci/legacy", Result: github.CheckTimedOut, URL: "javascript:alert(1)"},
		{Name: "lint", Result: github.CheckQueued},
		{Name: "CI / build", Result: github.CheckPassed, URL: "https://example.test/build"},
		{Name: "optional", Result: github.CheckNeutral},
	}
}

func checksHarness(test *testing.T, width, height int, edit func(*github.PullRequestDetail)) *harness {
	test.Helper()
	harness := newHarness(test, width, height)
	detail := testDetail()
	detail.CheckList = testChecks()
	detail.CheckTotal = len(detail.CheckList)
	if edit != nil {
		edit(&detail)
	}
	harness.client.SetPullRequest(detail)
	harness.run(harness.view.Init())
	return harness
}

func TestChecksSectionOrderAndWords(test *testing.T) {
	harness := checksHarness(test, 100, 40, nil)
	harness.press("l", "]")
	out := ansi.Strip(harness.render())
	var rows []string
	for _, line := range strings.Split(out, "\n") {
		if _, after, found := strings.Cut(line, "││ "); found {
			rows = append(rows, strings.TrimRight(strings.TrimSuffix(strings.TrimRight(after, " "), "│"), " "))
		}
	}
	want := []string{
		"Checks (8)",
		"✗ CI / check  failed",
		"✗ ci/legacy  timed out",
		"● deploy  running",
		"○ lint  queued",
		"✓ CI / build  passed",
		"✓ CI / vulnerabilities  passed",
		"- docs  skipped",
		"- optional  neutral",
	}
	for len(rows) > 0 && rows[0] != "Checks (8)" {
		rows = rows[1:]
	}
	if len(rows) < len(want) {
		test.Fatalf("too few rows:\n%s", out)
	}
	// The wanted rows carry two spaces between name and word; compare loosely.
	for i, row := range rows[:len(want)] {
		if strings.Join(strings.Fields(row), " ") != strings.Join(strings.Fields(want[i]), " ") {
			test.Errorf("row %d = %q, want %q", i, row, want[i])
		}
	}
}

func TestChecksSectionStyles(test *testing.T) {
	activeTheme := theme.New(true)
	harness := checksHarness(test, 100, 40, nil)
	harness.press("l", "]")
	out := harness.render()
	for _, want := range []string{
		activeTheme.PullRequestList.ChecksFailing.Render("✗"),
		activeTheme.PullRequestList.ChecksRunning.Render("●"),
		activeTheme.PullRequestList.ChecksPassing.Render("✓"),
		activeTheme.Muted.Render("-"),
	} {
		if !strings.Contains(out, want) {
			test.Errorf("render lacks %q", want)
		}
	}
}

func TestChecksSectionStates(test *testing.T) {
	tests := []struct {
		name string
		edit func(*github.PullRequestDetail)
		want []string
	}{
		{"empty", func(detail *github.PullRequestDetail) { detail.CheckList, detail.CheckTotal = nil, 0 }, []string{"Checks (0)", "No checks."}},
		{"over 100", func(detail *github.PullRequestDetail) { detail.CheckTotal = 130 }, []string{"Checks (130)", "and 122 more"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := checksHarness(test, 100, 40, testCase.edit)
			harness.press("l", "]")
			out := ansi.Strip(harness.render())
			for _, want := range testCase.want {
				if !strings.Contains(out, want) {
					test.Errorf("render lacks %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestChecksGolden(test *testing.T) {
	for _, width := range []int{100, 50} {
		test.Run(fmt.Sprint(width), func(test *testing.T) {
			harness := checksHarness(test, width, 30, nil)
			harness.press("l", "]")
			golden.Assert(test, fmt.Sprintf("checks_%d", width), harness.render(), false)
		})
	}
}

func TestChecksCommandPicker(test *testing.T) {
	harness := checksHarness(test, 100, 30, func(detail *github.PullRequestDetail) { detail.CheckTotal = 108 })
	command, handled := harness.view.Command("checks", "")
	if !handled {
		test.Fatal(":checks not handled")
	}
	harness.run(command)
	if !harness.view.CapturesKeys() {
		test.Fatal(":checks should open the picker")
	}
	out := ansi.Strip(harness.render())
	for _, want := range []string{"Checks", "✗ CI / check  failed", "and 100 more"} {
		if !strings.Contains(out, want) {
			test.Errorf("picker lacks %q:\n%s", want, out)
		}
	}
	// The first row is the failing CI / check.
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if diff := cmp.Diff([]string{"https://example.test/check"}, harness.opened); diff != "" {
		test.Errorf("opened (-want +got):\n%s", diff)
	}
	if harness.view.CapturesKeys() {
		test.Error("enter should close the picker")
	}
	harness.press("o")
	if got := harness.opened[len(harness.opened)-1]; got != testDetail().URL {
		test.Errorf("o opened %q, want the pull request", got)
	}
}

func TestChecksCommandRefusesUnsafeAndMissingLinks(test *testing.T) {
	harness := checksHarness(test, 100, 30, nil)
	command, _ := harness.view.Command("checks", "")
	harness.run(command)
	harness.send(tea.KeyPressMsg{Code: tea.KeyDown})
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter}) // ci/legacy: javascript: link
	if len(harness.opened) != 0 {
		test.Errorf("opened %v", harness.opened)
	}
	if len(harness.messages) == 0 {
		test.Error("no notice for a check without a usable link")
	}
}

func TestChecksCommandWithoutChecks(test *testing.T) {
	harness := checksHarness(test, 100, 30, func(detail *github.PullRequestDetail) { detail.CheckList, detail.CheckTotal = nil, 0 })
	command, handled := harness.view.Command("checks", "")
	if !handled {
		test.Fatal("not handled")
	}
	harness.run(command)
	if harness.view.CapturesKeys() {
		test.Error("picker opened without checks")
	}
	if _, ok := harness.messages[len(harness.messages)-1].(userinterface.InfoMessage); !ok {
		test.Errorf("messages = %v", harness.messages)
	}
}

func TestHeaderMergeState(test *testing.T) {
	activeTheme := theme.New(true)
	tests := []struct {
		state github.MergeState
		text  string
		style func(...string) string
	}{
		{github.MergeStateConflicts, "conflicts", activeTheme.FileDeleted.Render},
		{github.MergeStateBehind, "behind base", activeTheme.FileModified.Render},
		{github.MergeStateBlocked, "blocked", activeTheme.FileModified.Render},
		{github.MergeStateOther, "", nil},
	}
	for _, testCase := range tests {
		test.Run(testCase.text, func(test *testing.T) {
			harness := checksHarness(test, 200, 24, func(detail *github.PullRequestDetail) { detail.MergeState = testCase.state })
			out := harness.render()
			if testCase.style == nil {
				for _, word := range []string{"conflicts", "behind", "blocked"} {
					if strings.Contains(ansi.Strip(out), word) {
						test.Errorf("header mentions %q for a clean pull request", word)
					}
				}
				return
			}
			if !strings.Contains(out, testCase.style(testCase.text)) {
				test.Errorf("header lacks styled %q:\n%s", testCase.text, ansi.Strip(out))
			}
		})
	}
	test.Run("merged pull requests show nothing", func(test *testing.T) {
		harness := checksHarness(test, 200, 24, func(detail *github.PullRequestDetail) {
			detail.MergeState, detail.State = github.MergeStateBlocked, github.PullRequestStateMerged
		})
		if strings.Contains(ansi.Strip(harness.render()), "blocked") {
			test.Error("merged pull request shows a merge problem")
		}
	})
}

func TestReviewerEntries(test *testing.T) {
	at := testNow
	detail := &github.PullRequestDetail{
		PullRequest: github.PullRequest{Author: "alice"},
		ReviewRequests: []github.Reviewer{
			{Name: "erin"}, {Name: "core", IsTeam: true}, {Name: "bob"},
		},
		Reviews: []github.Review{
			{Author: "bob", State: github.ReviewApproved, SubmittedAt: at},
			{Author: "carol", State: github.ReviewChangesRequested, SubmittedAt: at},
			{Author: "carol", State: github.ReviewApproved, SubmittedAt: at.Add(time.Hour)},
			{Author: "dave", State: github.ReviewCommented, SubmittedAt: at},
			{Author: "alice", State: github.ReviewCommented, SubmittedAt: at},
			{Author: "frank", State: github.ReviewDismissed, SubmittedAt: at},
			{Author: "me", State: github.ReviewPending},
		},
	}
	want := []reviewerEntry{
		{"erin", markRequested},
		{"team core", markRequested},
		{"bob", markRequested},
		{"carol", markApproved},
		{"dave", markCommented},
	}
	if diff := cmp.Diff(want, reviewerEntries(detail), cmp.AllowUnexported(reviewerEntry{})); diff != "" {
		test.Errorf("entries (-want +got):\n%s", diff)
	}
}

func TestHeaderReviewerMarks(test *testing.T) {
	activeTheme := theme.New(true)
	harness := checksHarness(test, 240, 24, func(detail *github.PullRequestDetail) {
		detail.ReviewRequests = []github.Reviewer{{Name: "erin"}}
		detail.Reviews = []github.Review{
			{Author: "bob", State: github.ReviewApproved, SubmittedAt: testNow},
			{Author: "carol", State: github.ReviewChangesRequested, SubmittedAt: testNow},
			{Author: "dave", State: github.ReviewCommented, SubmittedAt: testNow},
		}
	})
	out := harness.render()
	if got := ansi.Strip(out); !strings.Contains(got, "reviewers: erin ?, bob ✓, carol ✗, dave •") {
		test.Errorf("header = %s", got)
	}
	styles := activeTheme.PullRequestList
	for _, want := range []string{styles.ReviewRequired.Render("?"), styles.Approved.Render("✓"), styles.ChangesRequested.Render("✗"), activeTheme.Muted.Render("•")} {
		if !strings.Contains(out, want) {
			test.Errorf("header lacks %q", want)
		}
	}
}

func TestStatusHeaderNarrow(test *testing.T) {
	for _, width := range []int{20, 40, 79} {
		harness := checksHarness(test, width, 20, func(detail *github.PullRequestDetail) { detail.MergeState = github.MergeStateConflicts })
		for i, line := range strings.Split(harness.render(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				test.Errorf("width %d: line %d is %d cells", width, i, got)
			}
		}
	}
}

func TestHostileChecksAndReviewers(test *testing.T) {
	harness := checksHarness(test, 200, 40, func(detail *github.PullRequestDetail) {
		detail.CheckList = []github.Check{
			{Name: "check" + hostile, Result: github.CheckFailed, URL: "https://example.test/\x1b]0;x\x07"},
			{Name: "workflow " + hostile + " / job", Result: github.CheckPassed},
		}
		detail.CheckTotal = 2
		detail.ReviewRequests = []github.Reviewer{{Name: "rev" + hostile}}
		detail.Reviews = []github.Review{{Author: "by" + hostile, State: github.ReviewApproved, SubmittedAt: testNow}}
	})
	harness.press("l", "]")
	assertTame(test, "checks", harness.render())
	command, _ := harness.view.Command("checks", "")
	harness.run(command)
	assertTame(test, "checks", harness.render())
}
