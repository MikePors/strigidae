package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

const (
	gapHead = "0123456789012345678901234567890123456789"
	gapPath = "internal/sync/client.go"
)

// gapText is a file of count lines, "line 1" to "line <count>".
func gapText(count int) string {
	var builder strings.Builder
	for number := 1; number <= count; number++ {
		fmt.Fprintf(&builder, "line %d\n", number)
	}
	return builder.String()
}

// gapHarness opens client.go, whose first hunk starts at line 10 and second at
// 42 (new side), with its text at the pull request's head set to count lines
// (none when count is 0). The cursor is on the gap above the first hunk, in
// the right pane.
func gapHarness(test *testing.T, head string, count int) *harness {
	test.Helper()
	harness := newHarness(test, 100, 40)
	detail := testDetail()
	detail.HeadSHA = head
	harness.client.SetPullRequest(detail)
	if count > 0 {
		harness.client.SetFileText(testRepository, head, gapPath, gapText(count))
	}
	harness.run(harness.view.Init())
	harness.press("h", "}", "l", "k")
	if !harness.pane().OnGap() {
		test.Fatalf("setup: the cursor is not on a gap:\n%s", harness.screen())
	}
	return harness
}

// fileTextCalls counts the FileText requests the client has seen.
func fileTextCalls(client *fake.Client) int {
	count := 0
	for _, call := range client.Calls() {
		if call.Method == fake.MethodFileText {
			count++
		}
	}
	return count
}

func TestEnterOnAGapReadsTheFileAndOpensIt(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	if !strings.Contains(harness.screen(), "9 hidden lines") {
		test.Fatalf("setup: no gap row:\n%s", harness.screen())
	}
	harness.press("enter")
	var requested []fake.Call
	for _, call := range harness.client.Calls() {
		if call.Method == fake.MethodFileText {
			requested = append(requested, call)
		}
	}
	if len(requested) != 1 || requested[0].CommitObjectIdentifier != gapHead || requested[0].Path != gapPath || requested[0].Repository != testRepository {
		test.Fatalf("FileText calls = %+v, want one for %s at the head", requested, gapPath)
	}
	screen := harness.screen()
	if strings.Contains(screen, "9 hidden lines") {
		test.Errorf("the gap row stayed:\n%s", screen)
	}
	for _, want := range []string{"line 1 ", "line 9 "} { // the top of the file, and the line above the hunk; rows are padded
		if !strings.Contains(screen, want) {
			test.Errorf("the screen lacks %q:\n%s", strings.TrimSpace(want), screen)
		}
	}
}

func TestTheFileIsReadOnceForAllItsGaps(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	harness.press("enter") // the gap above the first hunk opens whole
	harness.press("]", "k")
	if !harness.pane().OnGap() {
		test.Fatalf("setup: not on the gap between the hunks:\n%s", harness.screen())
	}
	harness.press("enter")
	if got := fileTextCalls(harness.client); got != 1 {
		test.Errorf("FileText called %d times, want 1", got)
	}
	if !strings.Contains(harness.screen(), "⋯ 2 hidden lines") {
		test.Errorf("the gap between the hunks did not open by a chunk:\n%s", harness.screen())
	}
}

// wantedText presses enter and returns the pane's request for the file's text.
func wantedText(test *testing.T, harness *harness) fileTextWantedMessage {
	test.Helper()
	_, command := harness.view.Update(keys.Message{Sequence: "enter"})
	for _, message := range messages(command) {
		if wanted, ok := message.(fileTextWantedMessage); ok {
			return wanted
		}
	}
	test.Fatal("enter on a gap did not ask for the file's text")
	return fileTextWantedMessage{}
}

func TestEnterOnAGapAsksOnceWhileTheTextIsOnItsWay(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	wanted := wantedText(test, harness)
	_, fetch := harness.view.Update(wanted)
	if got := messages(fetch); len(got) != 1 {
		test.Fatalf("the first request produced %v, want one read", got)
	}
	_, again := harness.view.Update(wanted)
	if got := messages(again); len(got) != 0 {
		test.Errorf("a second request while the first is on its way produced %v", got)
	}
}

func TestShownLinesAreNotCommentable(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	harness.press("enter") // the cursor lands on line 1
	if !harness.pane().Context().OnExpandedLine {
		test.Fatalf("setup: the cursor is not on a shown line:\n%s", harness.screen())
	}
	harness.press("c")
	if got := harness.lastInfo(); !strings.Contains(got, "not part of the diff") || !strings.Contains(got, "comment on it") {
		test.Errorf("c on a shown line says %q", got)
	}
	harness.press("V")
	if got := harness.lastInfo(); !strings.Contains(got, "not part of the diff") || !strings.Contains(got, "select lines") {
		test.Errorf("V on a shown line says %q", got)
	}
	if harness.view.visualActive() {
		test.Error("a selection started on a shown line")
	}
}

func TestGapStaysClosedWhenTheFileHasNoText(test *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup func(harness *harness)
	}{
		{"no text for the file", func(*harness) {}},
		{"binary or too large", func(harness *harness) {
			harness.client.SetFileTextUnavailable(testRepository, gapHead, gapPath)
		}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			harness := gapHarness(test, gapHead, 0)
			testCase.setup(harness)
			harness.press("enter")
			if got := harness.lastInfo(); !strings.Contains(got, "GitHub has no text for this file") {
				test.Errorf("status = %q", got)
			}
			if !strings.Contains(harness.screen(), "9 hidden lines") {
				test.Errorf("the gap opened:\n%s", harness.screen())
			}
		})
	}
}

func TestGapNeedsTheHeadOfTheDiff(test *testing.T) {
	harness := gapHarness(test, "", 0)
	harness.press("enter")
	if got := harness.lastInfo(); !strings.Contains(got, "head commit") {
		test.Errorf("status = %q", got)
	}
	if got := fileTextCalls(harness.client); got != 0 {
		test.Errorf("FileText called %d times with no head", got)
	}
}

func TestFileTextForAnotherFileIsDropped(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	_, read := harness.view.Update(wantedText(test, harness))
	harness.press("}") // another file, before the text arrives
	if harness.view.paneFile.Path == gapPath {
		test.Fatal("setup: still on the same file")
	}
	infos := len(harness.messages)
	harness.run(read)
	if len(harness.messages) != infos {
		test.Errorf("a late text said something: %+v", harness.messages[infos:])
	}
	harness.press("{") // back: the pane never got the text, so the gap is still closed
	if harness.view.paneFile.Path != gapPath || !strings.Contains(harness.screen(), "9 hidden lines") {
		test.Errorf("the late text opened a gap:\n%s", harness.screen())
	}
}

func TestFailedReadShowsOnlySafeText(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	harness.client.SetError(fake.MethodFileText, errors.New("get internal/\x1b]0;owned\x07evil\u009b31m.go"))
	harness.press("enter")
	var failure error
	for _, message := range harness.messages {
		if shown, ok := message.(userinterface.ErrorMessage); ok {
			failure = shown.Err
		}
	}
	if failure == nil {
		test.Fatal("a failed read said nothing")
	}
	if text := failure.Error(); strings.ContainsAny(text, "\x1b\x07\u009b") || !strings.Contains(text, "read the file's lines") {
		test.Errorf("the error carries terminal control text or lost its context: %q", text)
	}
}

func TestTextForOneGapNeverOpensAnother(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	_, read := harness.view.Update(wantedText(test, harness)) // enter on the gap above the first hunk
	harness.press("]", "]", "k")                              // meanwhile: onto the gap between the hunks
	if harness.pane().GapIndex() != 1 {
		test.Fatalf("setup: the cursor is not on the second gap:\n%s", harness.screen())
	}
	harness.run(read)
	screen := harness.screen()
	if !strings.Contains(screen, "9 hidden lines") || strings.Contains(screen, "line 23 ") {
		test.Errorf("the text opened a gap other than the one asked for:\n%s", screen)
	}
	if harness.pane().GapIndex() != 1 {
		test.Error("the cursor was moved")
	}
}

func TestRefreshBeforeTheTextArrivesOpensNothing(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	_, read := harness.view.Update(wantedText(test, harness))
	harness.view.filePane.SetDiff(harness.view.diff) // a refresh replaces the diff the request was made for
	harness.run(read)
	if !strings.Contains(harness.screen(), "9 hidden lines") {
		test.Errorf("a request from before the refresh opened a gap:\n%s", harness.screen())
	}
}

func TestReadsForTwoFilesDoNotRepeatEachOther(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	first := wantedText(test, harness)
	if _, fetch := harness.view.Update(first); len(messages(fetch)) != 1 {
		test.Fatal("setup: the first request did not start a read")
	}
	harness.press("}")
	second := fileTextWantedMessage{pane: harness.view.filePane, path: harness.view.paneFile.Path}
	if _, fetch := harness.view.Update(second); len(messages(fetch)) != 1 {
		test.Fatal("setup: the second file's request did not start a read")
	}
	harness.press("{")
	if _, fetch := harness.view.Update(first); len(messages(fetch)) != 0 {
		test.Error("asking again for a file already being read started another read")
	}
}

func TestSecondEnterAfterTheTextArrivedReadsNothing(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	first := wantedText(test, harness)
	second := wantedText(test, harness) // a second press before the first result
	_, read := harness.view.Update(first)
	harness.run(read)
	if _, fetch := harness.view.Update(second); len(messages(fetch)) != 0 {
		test.Error("a request that came after the text arrived started another read")
	}
	if got := fileTextCalls(harness.client); got != 1 {
		test.Errorf("FileText called %d times, want 1", got)
	}
}

func TestLateReadErrorIsNotSaidOffTheDiff(test *testing.T) {
	harness := gapHarness(test, gapHead, 60)
	harness.client.SetError(fake.MethodFileText, errors.New("boom"))
	_, read := harness.view.Update(wantedText(test, harness))
	harness.press("h", "g g") // back to the files list, up to Overview
	if harness.view.mode == contentFile {
		test.Fatal("setup: still showing a file")
	}
	infos := len(harness.messages)
	harness.run(read)
	if len(harness.messages) != infos {
		test.Errorf("a late error said something on the Overview: %+v", harness.messages[infos:])
	}
}
