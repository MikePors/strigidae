package workspace

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

const (
	firstCommentIdentifier  = "bobWhy retry here?"
	secondCommentIdentifier = "aliceHandles flaky links."
)

func (client blockingClient) AddReaction(ctx context.Context, _ string, _ github.ReactionContent) error {
	return client.wait(ctx)
}

// seedReactions gives the first comment of t-open a thumbs up (3, the viewer's),
// a laugh (1) and a heart (2, the viewer's), and refreshes the view.
func (inliner *inliner) seedReactions() {
	inliner.test.Helper()
	threads := testThreads()
	threads[0].Comments[0].Reactions = []github.ReactionSummary{
		{Content: github.ReactionThumbsUp, Count: 3, ViewerReacted: true},
		{Content: github.ReactionLaugh, Count: 1},
		{Content: github.ReactionHeart, Count: 2, ViewerReacted: true},
	}
	inliner.client.SetReviewThreads(testRepository, 7, threads)
	inliner.press("R")
}

// cursorLine is the diff pane's row under the cursor as the screen shows it.
func (inliner *inliner) cursorLine() string {
	rendered := ansi.Strip(inliner.diffView().Render(inliner.view.paneWidth, inliner.view.paneHeight))
	for line := range strings.SplitSeq(rendered, "\n") {
		if strings.HasPrefix(line, ">") {
			return line
		}
	}
	return ""
}

// toLineContaining puts the diff cursor, from the top, on the first row whose
// rendered text contains text; with text "" it is the first row that shows
// only the thread's bar, the blank row between two comments.
func (inliner *inliner) toLineContaining(text string) {
	inliner.test.Helper()
	inliner.cursorTo("a row showing "+text, func(*diffview.Model) bool {
		line := strings.TrimSpace(strings.TrimPrefix(inliner.cursorLine(), ">"))
		if text == "" {
			return line == "│"
		}
		return strings.Contains(line, text)
	})
}

// toComment puts the cursor on the body row of comment 0 or 1 of thread t-open.
func (inliner *inliner) toComment(commentIndex int) {
	inliner.test.Helper()
	inliner.toLineContaining([]string{"Why retry here?", "Handles flaky links."}[commentIndex])
}

func TestReactOnTheAuthorRowOfTheSecondComment(test *testing.T) {
	inliner := newInliner(test)
	inliner.toLineContaining("alice ·")
	inliner.react("rocket")
	calls := inliner.reactionCalls()
	if len(calls) != 1 || calls[0].SubjectIdentifier != secondCommentIdentifier {
		test.Fatalf("calls = %+v", calls)
	}
}

func TestReactOnTheBlankRowBetweenCommentsSaysSo(test *testing.T) {
	inliner := newInliner(test)
	inliner.toLineContaining("")
	inliner.react("rocket")
	if diff := cmp.Diff([]string{noticeNotOnComment}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(inliner.reactionCalls()) != 0 {
		test.Error("written")
	}
}

// toggleWithoutReload runs :react <name> and delivers the result of the
// write, but not the reload it asks for, so the screen shows only what the
// view itself recorded.
func (inliner *inliner) toggleWithoutReload(name string) {
	inliner.test.Helper()
	command, _ := inliner.view.Command("react", name)
	result, ok := command().(reactionToggledMessage)
	if !ok {
		inliner.test.Fatal("the write did not succeed")
	}
	inliner.view.Update(result)
}

func TestSecondReactDecidesFromTheFirst(test *testing.T) {
	inliner := newInliner(test)
	inliner.toComment(0)
	inliner.toggleWithoutReload("rocket") // adds
	inliner.messages = nil
	inliner.react("")
	if shown := inliner.shown(); !strings.Contains(shown, "🚀 rocket    1  ✓") {
		test.Fatalf("the picker does not show the reaction just added:\n%s", shown)
	}
	inliner.keyPress(tea.KeyEscape)
	inliner.toggleWithoutReload("rocket") // removes, not adds again
	want := []string{fake.MethodAddReaction, fake.MethodRemoveReaction}
	var got []string
	for _, call := range inliner.reactionCalls() {
		got = append(got, call.Method)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestReactPickerTitleShowsGhostForADeletedAuthor(test *testing.T) {
	inliner := newInliner(test)
	threads := testThreads()
	threads[0].Comments[0].Author = ""
	inliner.client.SetReviewThreads(testRepository, 7, threads)
	inliner.press("R")
	inliner.toComment(0)
	inliner.react("")
	if shown := inliner.shown(); !strings.Contains(shown, "React to ghost's comment") {
		test.Errorf("title does not use the thread's author text:\n%s", shown)
	}
}

func (inliner *inliner) react(name string) {
	inliner.test.Helper()
	inliner.messages = nil
	command, ok := inliner.view.Command("react", name)
	if !ok {
		inliner.test.Fatal("react not handled")
	}
	inliner.run(command)
}

func (inliner *inliner) reactionCalls() []fake.Call {
	var out []fake.Call
	for _, call := range inliner.client.Calls() {
		if call.Method == fake.MethodAddReaction || call.Method == fake.MethodRemoveReaction {
			out = append(out, call)
		}
	}
	return out
}

func (inliner *inliner) keyPress(code rune) {
	inliner.test.Helper()
	inliner.send(tea.KeyPressMsg{Code: code})
}

func (inliner *inliner) reactionsOf(thread string, commentIndex int) []github.ReactionSummary {
	inliner.test.Helper()
	return inliner.threadState(thread).Comments[commentIndex].Reactions
}

func TestReactionLabel(test *testing.T) {
	tests := []struct {
		content github.ReactionContent
		summary github.ReactionSummary
		want    string
	}{
		{github.ReactionThumbsUp, github.ReactionSummary{Count: 3, ViewerReacted: true}, "👍 +1        3  ✓"},
		{github.ReactionThumbsDown, github.ReactionSummary{}, "👎 -1"},
		{github.ReactionLaugh, github.ReactionSummary{Count: 1}, "😄 laugh     1"},
		{github.ReactionHeart, github.ReactionSummary{Count: 2, ViewerReacted: true}, "❤️ heart     2  ✓"},
	}
	for _, testCase := range tests {
		if got := reactionLabel(testCase.content, testCase.summary); got != testCase.want {
			test.Errorf("%s: label = %q, want %q", testCase.content, got, testCase.want)
		}
	}
}

func TestReactOpensPickerWithCountsAndMarks(test *testing.T) {
	inliner := newInliner(test)
	inliner.seedReactions()
	inliner.toComment(0)
	inliner.react("")
	if inliner.view.picker == nil || !inliner.view.CapturesKeys() {
		test.Fatal("no picker open")
	}
	shown := inliner.shown()
	for _, want := range []string{"React to bob's comment", "👍 +1        3  ✓", "😄 laugh     1", "❤️ heart     2  ✓", "👎 -1", "👀 eyes"} {
		if !strings.Contains(shown, want) {
			test.Errorf("picker lacks %q:\n%s", want, shown)
		}
	}
	if len(inliner.reactionCalls()) != 0 {
		test.Error("written by opening the picker")
	}
}

func TestReactPickerToggles(test *testing.T) {
	tests := []struct {
		name       string
		downs      int
		wantMethod string
		wantNotice string
		want       []github.ReactionSummary
	}{
		{"adds a reaction", 2, fake.MethodAddReaction, "reacted 😄", []github.ReactionSummary{
			{Content: github.ReactionThumbsUp, Count: 3, ViewerReacted: true},
			{Content: github.ReactionLaugh, Count: 2, ViewerReacted: true},
			{Content: github.ReactionHeart, Count: 2, ViewerReacted: true},
		}},
		{"removes the viewer's reaction", 0, fake.MethodRemoveReaction, "removed 👍", []github.ReactionSummary{
			{Content: github.ReactionThumbsUp, Count: 2},
			{Content: github.ReactionLaugh, Count: 1},
			{Content: github.ReactionHeart, Count: 2, ViewerReacted: true},
		}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seedReactions()
			inliner.toComment(0)
			inliner.react("")
			for range testCase.downs {
				inliner.keyPress(tea.KeyDown)
			}
			inliner.messages = nil
			inliner.keyPress(tea.KeyEnter)
			if inliner.view.picker != nil {
				test.Error("picker still open")
			}
			calls := inliner.reactionCalls()
			if len(calls) != 1 || calls[0].Method != testCase.wantMethod || calls[0].SubjectIdentifier != firstCommentIdentifier {
				test.Fatalf("calls = %+v", calls)
			}
			if diff := cmp.Diff([]string{testCase.wantNotice}, inliner.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.want, inliner.reactionsOf("t-open", 0)); diff != "" {
				test.Errorf("reactions after the reload (-want +got):\n%s", diff)
			}
			if inliner.view.writes != 0 {
				test.Errorf("writes = %d", inliner.view.writes)
			}
		})
	}
}

func TestReactPickerEscapeWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	inliner.toComment(0)
	inliner.react("")
	inliner.keyPress(tea.KeyDown)
	inliner.keyPress(tea.KeyEscape)
	if inliner.view.picker != nil || inliner.view.pickerReaction != nil || inliner.view.CapturesKeys() {
		test.Error("picker or its target left behind")
	}
	if len(inliner.reactionCalls()) != 0 {
		test.Errorf("calls = %+v", inliner.reactionCalls())
	}
}

func TestReactWithNameTogglesWithoutPicker(test *testing.T) {
	tests := []struct {
		name       string
		argument   string
		wantMethod string
		wantNotice string
	}{
		{"remove", "heart", fake.MethodRemoveReaction, "removed ❤️"},
		{"add", "rocket", fake.MethodAddReaction, "reacted 🚀"},
		{"plus one is the thumbs up", "+1", fake.MethodRemoveReaction, "removed 👍"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seedReactions()
			inliner.toComment(0)
			inliner.react(testCase.argument)
			if inliner.view.picker != nil {
				test.Error("picker opened")
			}
			calls := inliner.reactionCalls()
			if len(calls) != 1 || calls[0].Method != testCase.wantMethod {
				test.Fatalf("calls = %+v", calls)
			}
			if diff := cmp.Diff([]string{testCase.wantNotice}, inliner.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReactUnknownNameIsAnErrorThatWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	inliner.toComment(0)
	inliner.react("smile")
	err := inliner.lastErr()
	if err == nil || !strings.Contains(err.Error(), "smile") || !strings.Contains(err.Error(), "+1, -1, laugh, hooray, confused, heart, rocket, eyes") {
		test.Errorf("error = %v", err)
	}
	if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
		test.Error("something was written or opened")
	}
}

func TestReactTargetsTheCommentUnderTheCursor(test *testing.T) {
	inliner := newInliner(test)
	inliner.toComment(1)
	inliner.react("rocket")
	calls := inliner.reactionCalls()
	if len(calls) != 1 || calls[0].SubjectIdentifier != secondCommentIdentifier || calls[0].Reaction != github.ReactionRocket {
		test.Fatalf("calls = %+v", calls)
	}
}

func TestReactOffACommentSaysSoAndWritesNothing(test *testing.T) {
	tests := []struct {
		name   string
		moveTo func(inliner *inliner)
	}{
		{"diff line", func(inliner *inliner) { inliner.moveTo(github.SideRight, 10) }},
		{"thread header", func(inliner *inliner) {
			inliner.toThread("t-open") // the diff line; the header is the next row
			inliner.diffView().CursorDown(1)
		}},
		{"folded thread", func(inliner *inliner) { inliner.toThread("t-resolved") }},
		{"files pane", func(inliner *inliner) {
			inliner.toComment(0)
			inliner.press("h")
		}},
		{"overview", func(inliner *inliner) { inliner.press("h", "g g") }},
	}
	for _, testCase := range tests {
		for _, argument := range []string{"", "heart"} {
			test.Run(testCase.name+" "+argument, func(test *testing.T) {
				inliner := newInliner(test)
				testCase.moveTo(inliner)
				inliner.react(argument)
				if diff := cmp.Diff([]string{noticeNotOnComment}, inliner.infoMessages()); diff != "" {
					test.Errorf("info (-want +got):\n%s", diff)
				}
				if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
					test.Error("something was written or opened")
				}
			})
		}
	}
}

func TestReactRefusedWhileBusy(test *testing.T) {
	tests := []struct {
		name string
		busy func(inliner *inliner)
		want string
	}{
		{"review", func(inliner *inliner) { inliner.view.review.submitting = true }, "a review is being submitted"},
		{"merge", func(inliner *inliner) { inliner.view.merge.merging = true }, "a merge is in progress"},
		{"comment", func(inliner *inliner) { inliner.view.writes = 1 }, commentSavingText},
	}
	for _, testCase := range tests {
		for _, argument := range []string{"", "heart"} {
			test.Run(testCase.name+" "+argument, func(test *testing.T) {
				inliner := newInliner(test)
				inliner.toComment(0)
				testCase.busy(inliner)
				writes := inliner.view.writes
				inliner.react(argument)
				if diff := cmp.Diff([]string{testCase.want}, inliner.infoMessages()); diff != "" {
					test.Errorf("info (-want +got):\n%s", diff)
				}
				if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil || inliner.view.writes != writes {
					test.Error("something was written or opened")
				}
			})
		}
	}
}

func TestReactTimeoutSaysItMayHaveGoneThrough(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.client = blockingClient{Client: inliner.client}
	inliner.view.timeouts = shortTimeouts
	inliner.toComment(0)
	inliner.react("rocket")
	err := inliner.lastErr()
	if err == nil || !strings.Contains(err.Error(), "it may have gone through") {
		test.Errorf("error = %v, want it to say the write may have gone through", err)
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes = %d, want 0", inliner.view.writes)
	}
}

func TestReactFailureShowsTheError(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetError(fake.MethodAddReaction, github.ErrForbidden)
	inliner.toComment(0)
	inliner.react("rocket")
	if err := inliner.lastErr(); err == nil {
		test.Error("no error shown")
	}
	if inliner.view.writes != 0 || len(inliner.infoMessages()) != 0 {
		test.Errorf("writes %d, info %v", inliner.view.writes, inliner.infoMessages())
	}
}

func TestReactPickerTitleCleansTheAuthor(test *testing.T) {
	inliner := newInliner(test)
	threads := testThreads()
	threads[0].Comments[0].Author = "bo\x1b[31mb"
	inliner.client.SetReviewThreads(testRepository, 7, threads)
	inliner.press("R")
	inliner.toComment(0)
	inliner.react("")
	if inliner.view.picker == nil {
		test.Fatal("no picker")
	}
	if shown := inliner.render(); strings.Contains(shown, "\x1b[31mb") {
		test.Errorf("author escape reached the screen: %q", shown)
	}
}
