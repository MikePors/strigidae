package workspace

import (
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// threadVisits presses sequence presses times from where the cursor is and
// returns the author each landing shows ("" when the cursor is not on a
// thread header).
func threadVisits(harness *harness, sequence string, presses int) []string {
	harness.test.Helper()
	var visited []string
	for range presses {
		harness.press(sequence)
		row := harness.cursorRow()
		landed := ""
		for _, author := range []string{"carol", "bob", "dave"} {
			if strings.Contains(row, author) {
				landed = author
			}
		}
		visited = append(visited, landed)
	}
	return visited
}

func TestSkipResolvedOption(test *testing.T) {
	testCases := []struct {
		name    string
		values  options.Values
		forward []string
	}{
		{"off visits resolved threads", nil, []string{"carol", "bob", "dave"}},
		{"on skips resolved threads", options.Values{options.SkipResolved: true}, []string{"bob", "dave", "dave"}},
		{"with hideresolved", options.Values{options.SkipResolved: true, options.HideResolved: true}, []string{"bob", "dave", "dave"}},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			harness := threadHarness(test, 110, 40)
			harness.selectFile()
			harness.send(userinterface.OptionsMessage{Values: testCase.values})
			harness.press("g g")
			got := threadVisits(harness, ")", 3)
			if strings.Join(got, ",") != strings.Join(testCase.forward, ",") {
				test.Errorf("visits = %v, want %v", got, testCase.forward)
			}
		})
	}
}

func TestSkipResolvedBackwardAndCounts(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	harness.send(userinterface.OptionsMessage{Values: options.Values{options.SkipResolved: true}})

	harness.press("G")
	harness.press("(") // dave, the unresolved outdated thread, is visited
	harness.press("(")
	if !strings.Contains(harness.cursorRow(), "bob") {
		test.Errorf("second ( should land on bob, at %q", harness.cursorRow())
	}
	harness.press("(") // carol is resolved: nothing unresolved above
	if !strings.Contains(harness.cursorRow(), "bob") {
		test.Errorf("( with no unresolved thread above moved to %q", harness.cursorRow())
	}

	harness.press("g g")
	harness.pressCounted(")", 2) // counts only threads landed on: bob, then dave
	if !strings.Contains(harness.cursorRow(), "dave") {
		test.Errorf("2) should land on the second unresolved thread, at %q", harness.cursorRow())
	}
}

func TestSkipResolvedNoUnresolvedThreads(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	var all []github.ReviewThread
	for _, thread := range testThreads() {
		thread.IsResolved = true
		all = append(all, thread)
	}
	harness.client.SetReviewThreads(testRepository, 7, all)
	harness.press("R")
	harness.selectFile()
	harness.send(userinterface.OptionsMessage{Values: options.Values{options.SkipResolved: true}})
	harness.press("g g")
	before := harness.cursorRow()
	harness.messages = nil
	harness.press(")")
	if harness.cursorRow() != before {
		test.Errorf(") moved the cursor from %q to %q", before, harness.cursorRow())
	}
	if len(harness.messages) == 0 {
		test.Fatal("no notice shown")
	}
	last, ok := harness.messages[len(harness.messages)-1].(userinterface.InfoMessage)
	if !ok || last.Text != "no unresolved threads" {
		test.Errorf("messages = %v, want the no unresolved threads notice", harness.messages)
	}
}

func TestSkipResolvedAppliesToOpenWorkspace(test *testing.T) {
	harness := threadHarness(test, 110, 40)
	harness.selectFile()
	harness.press("g g")
	harness.send(userinterface.OptionsMessage{Values: options.Values{options.SkipResolved: true}})
	harness.press(")")
	if strings.Contains(harness.cursorRow(), "carol") {
		test.Error("option did not take effect in an open workspace")
	}
	if !strings.Contains(harness.screen(), "[resolved]") {
		test.Error("resolved thread should still be shown")
	}
}

func TestSkipResolvedNoticeOnlyWhenNothingToVisit(test *testing.T) {
	testCases := []struct {
		name       string
		values     options.Values
		presses    int
		wantNotice bool
	}{
		{"option on, landing on a thread", options.Values{options.SkipResolved: true}, 1, false},
		{"option on, past the last thread", options.Values{options.SkipResolved: true}, 3, true},
		{"option off, past the last thread", nil, 4, false},
	}
	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			harness := threadHarness(test, 110, 40)
			harness.selectFile()
			harness.send(userinterface.OptionsMessage{Values: testCase.values})
			harness.press("g g")
			for range testCase.presses {
				harness.messages = nil
				harness.press(")")
			}
			notice := false
			for _, message := range harness.messages {
				if info, ok := message.(userinterface.InfoMessage); ok && info.Text == "no unresolved threads" {
					notice = true
				}
			}
			if notice != testCase.wantNotice {
				test.Errorf("notice = %v, want %v (messages %v)", notice, testCase.wantNotice, harness.messages)
			}
		})
	}
}
