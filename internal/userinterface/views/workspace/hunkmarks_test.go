package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// client.go in testdata/pull_request.diff has two hunks.
const (
	firstHunkHeader  = "@@ -10,8 +10,10 @@ import ("
	secondHunkHeader = "@@ -40,5 +42,7 @@ func (c *Client) sync"
)

// hunkHarness opens client.go with the diff pane focused, cursor on the
// first hunk's header.
func hunkHarness(test *testing.T) *harness {
	test.Helper()
	harness := loaded(test, 100, 30)
	harness.press("j", "l")
	return harness
}

func (harness *harness) lastInfo() string {
	for i := len(harness.messages) - 1; i >= 0; i-- {
		if info, ok := harness.messages[i].(userinterface.InfoMessage); ok {
			return info.Text
		}
	}
	return ""
}

func (harness *harness) screen() string { return ansi.Strip(harness.render()) }

// reviewedHeaders lists the hunk headers on screen that carry the reviewed
// marker.
func (harness *harness) reviewedHeaders() []string {
	var marked []string
	for _, header := range []string{firstHunkHeader, secondHunkHeader} {
		if strings.Contains(harness.screen(), "✓ "+header) {
			marked = append(marked, header)
		}
	}
	return marked
}

func TestMarkHunkReviewed(test *testing.T) {
	harness := hunkHarness(test)
	if screen := harness.screen(); strings.Contains(screen, "hunks reviewed") || strings.Contains(screen, "✓ @@") {
		test.Fatalf("marks shown before any hunk was marked:\n%s", screen)
	}

	calls := len(harness.client.Calls())
	harness.press("m")
	if got := harness.reviewedHeaders(); len(got) != 1 || got[0] != firstHunkHeader {
		test.Errorf("marked headers = %q, want the first hunk", got)
	}
	if screen := harness.screen(); !strings.Contains(screen, "client.go · 1/2 hunks reviewed") {
		test.Errorf("title lacks the count:\n%s", screen)
	}

	if got := harness.lastInfo(); got != "hunk 1/2 reviewed" {
		test.Errorf("status = %q, want hunk 1/2 reviewed", got)
	}

	harness.press("]", "j", "m") // from a line inside the second hunk
	if got := harness.reviewedHeaders(); len(got) != 2 {
		test.Errorf("marked headers = %q, want both", got)
	}
	if screen := harness.screen(); !strings.Contains(screen, "2/2 hunks reviewed") {
		test.Errorf("title lacks 2/2:\n%s", screen)
	}

	harness.press("m")
	if got := harness.lastInfo(); got != "hunk 2/2 not reviewed" {
		test.Errorf("status = %q, want hunk 2/2 not reviewed", got)
	}
	if got := harness.reviewedHeaders(); len(got) != 1 || got[0] != firstHunkHeader {
		test.Errorf("after unmarking, marked headers = %q, want the first hunk", got)
	}
	if got := harness.client.Calls()[calls:]; len(got) != 0 {
		test.Errorf("marking hunks called GitHub: %+v", got)
	}
}

func TestMarkHunkFromFilesPane(test *testing.T) {
	harness := hunkHarness(test)
	harness.press("h", "m") // the files pane has focus; the diff cursor is still on hunk 1
	if got := harness.reviewedHeaders(); len(got) != 1 {
		test.Errorf("marked headers = %q, want the first hunk", got)
	}
}

func TestMarkHunkWithoutHunk(test *testing.T) {
	harness := loaded(test, 100, 30) // Overview selected
	harness.press("m")
	last, ok := harness.messages[len(harness.messages)-1].(userinterface.InfoMessage)
	if !ok || last.Text != "no hunk at the cursor" {
		test.Errorf("last message = %#v, want the no-hunk notice", harness.messages[len(harness.messages)-1])
	}
	if len(harness.view.hunkMarks.reviewed) != 0 {
		test.Error("a mark was set with no hunk at the cursor")
	}
}

// TestMarkHunkOnOverview leaves a file for the Overview: the diff pane still
// holds the file's cursor, but it is not on screen, so m marks nothing.
func TestMarkHunkOnOverview(test *testing.T) {
	harness := hunkHarness(test)
	harness.press("h", "k", "m")
	if len(harness.view.hunkMarks.reviewed) != 0 {
		test.Error("m marked a hunk of a file that is not shown")
	}
}

// TestHunkMarksFollowContent pushes a change that edits the first hunk and
// shifts the second: the second keeps its mark, the first loses it.
func TestHunkMarksFollowContent(test *testing.T) {
	harness := hunkHarness(test)
	harness.press("m", "]", "m")

	pushed := strings.Replace(readDiff(test), "@@ -40,5 +42,7 @@", "@@ -40,5 +43,7 @@", 1)
	pushed = strings.Replace(pushed, "@@ -10,8 +10,10 @@ import (", "@@ -10,8 +10,11 @@ import (\n+\t\"errors\"", 1)
	if pushed == readDiff(test) {
		test.Fatal("setup: the pushed diff is unchanged")
	}
	harness.client.SetDiff(testRepository, 7, pushed)
	harness.press("R")

	screen := harness.screen()
	if strings.Contains(screen, "✓ @@ -10,8 +10,11") {
		test.Errorf("the edited hunk kept its mark:\n%s", screen)
	}
	if !strings.Contains(screen, "✓ @@ -40,5 +43,7 @@") || !strings.Contains(screen, "1/2 hunks reviewed") {
		test.Errorf("the moved but unchanged hunk lost its mark:\n%s", screen)
	}
}

func TestIdenticalHunksHaveSeparateMarks(test *testing.T) {
	var cache fingerprintCache
	file := parseOne(test, "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,1 +1,1 @@\n-a\n+b\n@@ -9,1 +9,1 @@\n-a\n+b\n")
	got := cache.fingerprintsOf(file)
	if len(got) != 2 || got[0] == got[1] {
		test.Errorf("fingerprints = %q, want two different ones", got)
	}
}

func parseOne(test *testing.T, text string) *diff.File {
	test.Helper()
	parsed, err := diff.Parse(text)
	if err != nil || len(parsed.Files) != 1 {
		test.Fatalf("parse: %v", err)
	}
	return parsed.Files[0]
}

func TestHunkMarksShared(test *testing.T) {
	first := hunkHarness(test)
	first.press("m")

	shared := hunkHarness(test)
	WithHunkMarks(first.view.hunkMarks)(shared.view)
	if got := shared.reviewedHeaders(); len(got) != 1 {
		test.Errorf("a workspace sharing the marks shows %q, want the first hunk marked", got)
	}

	if got := hunkHarness(test).reviewedHeaders(); len(got) != 0 {
		test.Errorf("a workspace with its own marks shows %q, want none", got)
	}
}

func TestHunkMarkKeyInHelp(test *testing.T) {
	harness := loaded(test, 100, 30)
	for _, group := range harness.view.FullHelp() {
		for _, binding := range group {
			if binding.Help().Key == "m" {
				return
			}
		}
	}
	test.Error("m is not in the help")
}

// TestMarkHunkWhileDiffHidden: in a narrow terminal with the Files pane shown
// the diff is not on screen, so there is no hunk to mark.
func TestMarkHunkWhileDiffHidden(test *testing.T) {
	harness := loaded(test, 60, 20)
	harness.press("j", "m")
	if strings.Contains(harness.screen(), "@@") {
		test.Fatal("setup: the diff is on screen")
	}
	if got := harness.lastInfo(); got != "focus the diff to mark the hunk" {
		test.Errorf("status = %q, want the focus notice", got)
	}
	if len(harness.view.hunkMarks.reviewed) != 0 {
		test.Error("a hunk was marked")
	}
}

// TestHunkProgressSurvivesLongPath: the title shortens a long path from the
// left so the progress stays visible.
func TestHunkProgressSurvivesLongPath(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j", "j", "j", "j", "l", "m") // the renamed file with the long path
	title := strings.SplitN(harness.screen(), "\n", 4)[2]
	if !strings.Contains(title, "· 1/1 hunks reviewed") || !strings.Contains(title, "…") || !strings.Contains(title, "retry-policy-and-backoff.md") {
		test.Errorf("title row = %q, want a left-shortened path and the progress", title)
	}
	if got := harness.view.rightTitle(30); ansi.StringWidth(got) > 30 || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, " · 1/1 hunks reviewed") {
		test.Errorf("rightTitle(30) = %q (%d cells), want the path shortened to fit the progress", got, ansi.StringWidth(got))
	}
}

func TestFingerprintsCachedPerFile(test *testing.T) {
	harness := hunkHarness(test)
	harness.press("m")
	harness.render()
	harness.render()
	if got := harness.view.hunkFingerprints.computed; got != 1 {
		test.Errorf("hashed %d times for one file, want once", got)
	}
	harness.press("{", "}", "}") // another file, then back
	harness.render()
	if got := harness.view.hunkFingerprints.computed; got < 2 {
		test.Errorf("hashed %d times after changing file, want a fresh hash", got)
	}
}
