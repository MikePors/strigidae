package workspace

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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
	if diff := cmp.Diff([]string{"move onto a comment to react to it"}, inliner.infoMessages()); diff != "" {
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

// The highlight is checked before the reload runs, so it must come from the
// local update alone.
func TestReactHighlightFollowsTheToggle(test *testing.T) {
	tests := []struct {
		name     string
		argument string
		want     []string
	}{
		{"added reaction turns to the own style", "laugh", []string{"OWN:👍 3", "OWN:😄 2", "OWN:❤️ 2"}},
		{"removed reaction turns muted", "+1", []string{"MUTED:👍 2", "MUTED:😄 1", "OWN:❤️ 2"}},
	}
	activeTheme := theme.New(true)
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seedReactions()
			inliner.toComment(0)
			inliner.toggleWithoutReload(testCase.argument)
			out := inliner.diffView().Render(inliner.view.paneWidth, inliner.view.paneHeight)
			for _, part := range testCase.want {
				style, text, _ := strings.Cut(part, ":")
				rendered := activeTheme.Muted.Render(text)
				if style == "OWN" {
					rendered = activeTheme.OwnReaction.Render(text)
				}
				if !strings.Contains(out, rendered) {
					test.Errorf("want %s:\n%q", part, out)
				}
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
	}
	for _, testCase := range tests {
		for _, argument := range []string{"", "heart"} {
			test.Run(testCase.name+" "+argument, func(test *testing.T) {
				inliner := newInliner(test)
				testCase.moveTo(inliner)
				inliner.react(argument)
				if diff := cmp.Diff([]string{"move onto a comment to react to it"}, inliner.infoMessages()); diff != "" {
					test.Errorf("info (-want +got):\n%s", diff)
				}
				if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
					test.Error("something was written or opened")
				}
			})
		}
	}
}

func TestReactOnOverviewOpensTargetPicker(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	detail.Comments[1].Identifier = "issue-comment-2"
	inliner.client.SetPullRequest(detail)
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("")
	if inliner.view.picker == nil || len(inliner.view.pickerComment.targets) != 3 {
		test.Fatal("overview target picker did not open")
	}
	if got := []commentKind{inliner.view.pickerComment.targets[0].kind, inliner.view.pickerComment.targets[1].kind, inliner.view.pickerComment.targets[2].kind}; !cmp.Equal(got, []commentKind{commentDescription, commentTopLevel, commentTopLevel}) {
		test.Fatalf("target order = %v", got)
	}
	if got := commentTargetLabel(inliner.view.pickerComment.targets[1], pickerLabelWidth(inliner.view.width)); !strings.Contains(got, "bob") || !strings.Contains(got, "Thanks!") {
		test.Errorf("target label lacks author/body distinction: %q", got)
	}
}

func TestNamedOverviewReactionValidatesThenOpensTargetPicker(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	inliner.view.detail = &detail
	inliner.view.diff = DiffState{Err: context.DeadlineExceeded}
	inliner.press("h", "g g", "l")
	inliner.react("rocket")
	if inliner.view.picker == nil || inliner.view.pickerComment.call.arguments != "rocket" || len(inliner.reactionCalls()) != 0 {
		test.Fatal("named reaction did not wait for a target choice")
	}
	inliner.keyPress(tea.KeyEscape)
	if inliner.view.picker != nil || len(inliner.reactionCalls()) != 0 {
		test.Fatal("cancel wrote a reaction")
	}
}

func TestOverviewReactionRequiresLoadedDetail(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.detail = nil
	inliner.press("h", "g g", "l")
	inliner.react("")
	if inliner.view.picker != nil || len(inliner.reactionCalls()) != 0 {
		test.Fatal("opened or wrote before the pull request loaded")
	}
	if !strings.Contains(strings.Join(inliner.infoMessages(), " "), "pull request not loaded") {
		test.Errorf("infos = %v", inliner.infoMessages())
	}
}

func TestOverviewReactionRefusesRemovedTargetAtAdmission(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("rocket")
	refreshed := *inliner.view.detail
	refreshed.Comments = nil
	inliner.view.detail = &refreshed
	inliner.keyPress(tea.KeyDown)
	inliner.keyPress(tea.KeyEnter)
	if len(inliner.reactionCalls()) != 0 {
		test.Fatalf("stale target was written: %+v", inliner.reactionCalls())
	}
	if !strings.Contains(strings.Join(inliner.infoMessages(), " "), "no longer exists") {
		test.Errorf("infos = %v", inliner.infoMessages())
	}
}

func TestOverviewReactionPickerStagesCancelAndToggle(test *testing.T) {
	newOverviewInliner := func(test *testing.T) *inliner {
		inliner := newInliner(test)
		detail := testDetail()
		detail.Comments[0].Identifier = "issue-comment-1"
		inliner.client.SetPullRequest(detail)
		inliner.view.detail = &detail
		inliner.press("h", "g g", "l")
		return inliner
	}
	test.Run("cancel target picker", func(test *testing.T) {
		inliner := newOverviewInliner(test)
		inliner.react("")
		inliner.keyPress(tea.KeyEscape)
		if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
			test.Fatal("target cancellation wrote or left picker open")
		}
	})
	test.Run("cancel reaction picker", func(test *testing.T) {
		inliner := newOverviewInliner(test)
		inliner.react("")
		inliner.keyPress(tea.KeyEnter)
		inliner.keyPress(tea.KeyEscape)
		if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
			test.Fatal("reaction cancellation wrote or left picker open")
		}
	})
	test.Run("named picker toggles selected issue comment", func(test *testing.T) {
		inliner := newOverviewInliner(test)
		inliner.react("rocket")
		inliner.keyPress(tea.KeyDown) // top-level comment, after description
		inliner.keyPress(tea.KeyEnter)
		calls := inliner.reactionCalls()
		if len(calls) != 1 || calls[0].SubjectIdentifier != "issue-comment-1" || calls[0].Reaction != github.ReactionRocket {
			test.Fatalf("calls = %+v", calls)
		}
	})
}

func TestOverviewReactionUsesSelectedCommentAfterDiffCursorMoves(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	inliner.client.SetPullRequest(detail)
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("")
	inliner.keyPress(tea.KeyDown) // select the top-level comment
	inliner.keyPress(tea.KeyEnter)
	inliner.toComment(1)           // the open picker must keep the selected Overview context
	inliner.keyPress(tea.KeyEnter) // like
	calls := inliner.reactionCalls()
	if len(calls) != 1 || calls[0].SubjectIdentifier != "issue-comment-1" || calls[0].Reaction != github.ReactionThumbsUp {
		test.Fatalf("calls = %+v", calls)
	}
	if inliner.lastErr() != nil {
		test.Fatalf("reaction failed: %v", inliner.lastErr())
	}
}

func TestOverviewReactionPickerShowsCountsAndViewerMark(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Reactions = []github.ReactionSummary{{Content: github.ReactionHeart, Count: 4, ViewerReacted: true}}
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("")
	inliner.keyPress(tea.KeyEnter)
	if shown := inliner.shown(); !strings.Contains(shown, "❤️ heart") || !strings.Contains(shown, "4  ✓") {
		test.Fatalf("shared reaction picker omitted count or viewer mark:\n%s", shown)
	}
}

func TestOverviewReactionLabelsCleanHostileText(test *testing.T) {
	target := commentTarget{kind: commentTopLevel, author: "x\x1b[31m", body: "hello\u202e world\x9b"}
	label := commentTargetLabel(target, 56)
	if strings.ContainsRune(label, '\x1b') || strings.ContainsRune(label, '\u202e') || strings.ContainsRune(label, '\u009b') {
		test.Errorf("unsafe label %q", label)
	}
}

func TestOverviewReactionTargetPickerGolden(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	detail.Comments[1].Identifier = "issue-comment-2"
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("")
	golden.Assert(test, "overview_react_targets", inliner.render(), false)
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

func TestOverviewReactionUpdatesBeforeReloadAndUsesFreshState(test *testing.T) {
	for _, state := range noDiffStates() {
		for _, commentIndex := range []int{-1, 0} {
			test.Run(state.name+"/"+map[bool]string{true: "description", false: "comment"}[commentIndex < 0], func(test *testing.T) {
				inliner := newInliner(test)
				detail := testDetail()
				detail.Comments[0].Identifier = "issue-comment-1"
				inliner.client.SetPullRequest(detail)
				inliner.view.detail = &detail
				inliner.press("h", "g g", "l")
				state.removeDiff(inliner)
				for _, wantMethod := range []string{fake.MethodAddReaction, fake.MethodRemoveReaction} {
					inliner.react("")
					if commentIndex >= 0 {
						inliner.keyPress(tea.KeyDown)
					}
					inliner.keyPress(tea.KeyEnter)
					if inliner.view.pickerReaction == nil {
						test.Fatal("no reaction picker")
					}
					// Run the write and deliver its result, leaving reload commands unexecuted.
					command := inliner.view.pickerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
					result, ok := command().(reactionToggledMessage)
					if !ok {
						test.Fatal("write failed")
					}
					inliner.view.Update(result)
					identifier := detail.Identifier
					if commentIndex >= 0 {
						identifier = "issue-comment-1"
					}
					target, found := inliner.view.resolveCommentTarget(identifier)
					if !found || (len(target.reactions) > 0) != (wantMethod == fake.MethodAddReaction) {
						test.Fatalf("immediate reactions = %+v", target.reactions)
					}
					calls := inliner.reactionCalls()
					if calls[len(calls)-1].Method != wantMethod {
						test.Fatalf("calls = %+v", calls)
					}
				}
				if len(detail.Reactions) != 0 || len(detail.Comments[0].Reactions) != 0 {
					test.Fatal("original snapshot mutated")
				}
			})
		}
	}
}

func TestOverviewReactionPickerRechecksState(test *testing.T) {
	for _, stage := range []string{"target", "reaction"} {
		for _, busy := range busyCases() {
			test.Run(stage+"/"+busy.name, func(test *testing.T) {
				inliner := newInliner(test)
				inliner.press("h", "g g", "l")
				inliner.react("")
				if stage == "reaction" {
					inliner.keyPress(tea.KeyEnter)
				}
				busy.apply(inliner)
				inliner.keyPress(tea.KeyEnter)
				if len(inliner.reactionCalls()) != 0 || inliner.view.picker != nil {
					test.Fatal("busy choice wrote or retained picker")
				}
			})
		}
	}
}

func TestOverviewReactionChoiceUsesLatestReactions(test *testing.T) {
	inliner := newInliner(test)
	inliner.press("h", "g g", "l")
	inliner.react("")
	inliner.keyPress(tea.KeyEnter)
	detail := *inliner.view.detail
	detail.Reactions = []github.ReactionSummary{{Content: github.ReactionThumbsUp, Count: 1, ViewerReacted: true}}
	inliner.client.SetPullRequest(detail)
	inliner.view.detail = &detail
	inliner.keyPress(tea.KeyEnter)
	calls := inliner.reactionCalls()
	if len(calls) != 1 || calls[0].Method != fake.MethodRemoveReaction {
		test.Fatalf("calls = %+v", calls)
	}
}

func BenchmarkOverviewReactionTargets(benchmark *testing.B) {
	detail := testDetail()
	detail.Comments = make([]github.IssueComment, 1000)
	for i := range detail.Comments {
		detail.Comments[i] = github.IssueComment{Identifier: fmt.Sprintf("comment-%d", i), Author: "reviewer", Body: strings.Repeat("discussion ", 100)}
	}
	view := &View{detail: &detail, theme: theme.New(true), width: 100}
	benchmark.Run("open picker", func(benchmark *testing.B) {
		benchmark.ReportAllocs()
		for benchmark.Loop() {
			view.openCommentTargetPicker(view.entryNamed("react"), invocation{})
		}
	})
	benchmark.Run("resolve last target", func(benchmark *testing.B) {
		benchmark.ReportAllocs()
		for benchmark.Loop() {
			if _, found := view.resolveCommentTarget("comment-999"); !found {
				benchmark.Fatal("missing target")
			}
		}
	})
}

func TestOverviewReactionReloadsIgnoreOlderResults(test *testing.T) {
	inliner := newInliner(test)
	inliner.press("h", "g g", "l")
	detail := *inliner.view.detail
	detail.Reactions = []github.ReactionSummary{{Content: github.ReactionHeart, Count: 1, ViewerReacted: true}}
	inliner.client.SetPullRequest(detail)
	older := inliner.view.reloadPullRequest()()
	detail.Reactions = nil
	inliner.client.SetPullRequest(detail)
	newer := inliner.view.reloadPullRequest()()
	inliner.send(newer)
	inliner.send(older)
	if len(inliner.view.detail.Reactions) != 0 {
		test.Fatal("older add refresh overwrote newer removal")
	}
	inliner.react("heart")
	inliner.keyPress(tea.KeyEnter)
	calls := inliner.reactionCalls()
	if len(calls) != 1 || calls[0].Method != fake.MethodAddReaction {
		test.Fatalf("toggle used stale state: %+v", calls)
	}
}

func TestOverviewReactionLabelsFitNarrowPicker(test *testing.T) {
	for _, width := range []int{32, 48, 80} {
		inliner := newInliner(test)
		inliner.view.width = width
		inliner.press("h", "g g", "l")
		inliner.react("")
		rendered := ansi.Strip(inliner.view.picker.Render(width, 20))
		if !strings.Contains(rendered, "Description · alice") {
			test.Fatalf("width %d lost target prefix:\n%s", width, rendered)
		}
	}
}

func TestReactReportsUnloadedDiffBeforeHiddenCursor(test *testing.T) {
	for _, name := range []string{"", "heart"} {
		inliner := newInliner(test)
		inliner.toComment(0)
		inliner.press("h")
		inliner.view.width = 20
		inliner.view.diffPending = true
		inliner.react(name)
		if diff := cmp.Diff([]string{"diff not loaded"}, inliner.infoMessages()); diff != "" {
			test.Fatal(diff)
		}
		if inliner.view.picker != nil || len(inliner.reactionCalls()) != 0 {
			test.Fatal("used stale diff cursor")
		}
	}
}

func TestReactionMenuRefusesDeletedOverviewComment(test *testing.T) {
	inliner := newInliner(test)
	detail := testDetail()
	detail.Comments[0].Identifier = "issue-comment-1"
	inliner.view.detail = &detail
	inliner.press("h", "g g", "l")
	inliner.react("")
	inliner.keyPress(tea.KeyDown)
	inliner.keyPress(tea.KeyEnter)
	detail.Comments = nil
	inliner.keyPress(tea.KeyEnter)
	if len(inliner.reactionCalls()) != 0 || inliner.view.CapturesKeys() {
		test.Fatal("deleted target was written or retained keys")
	}
	if diff := cmp.Diff([]string{"comment no longer exists; refresh and try again"}, inliner.infoMessages()); diff != "" {
		test.Fatal(diff)
	}
}
