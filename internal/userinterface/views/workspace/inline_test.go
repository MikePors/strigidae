package workspace

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// Lines of internal/sync/client.go in testdata/pull_request.diff used below: new 10 is
// a context line, old 14 is the deleted `func Sync`, old 15 the deleted
// `return`, and new 14..16 are added lines.
const clientPath = "internal/sync/client.go"

// inliner is a reviewer with the diff file selected and the right pane focused.
type inliner struct{ *composer }

func newInliner(test *testing.T) *inliner {
	composer := newReviewer(test)
	composer.selectFile()
	return &inliner{composer}
}

// diffView is the diff component behind the right pane.
func (inliner *inliner) diffView() *diffview.Model { return inliner.view.filePane.(*diffPane).diffView }

// cursorTo moves the diff cursor, from the top, to the first row where found
// holds.
func (inliner *inliner) cursorTo(what string, found func(diffView *diffview.Model) bool) {
	inliner.test.Helper()
	diffView := inliner.diffView()
	diffView.GotoTop()
	for range 200 {
		if found(diffView) {
			return
		}
		diffView.CursorDown(1)
	}
	inliner.test.Fatalf("cursor cannot reach %s", what)
}

// moveTo puts the diff cursor on the line with the given number on side.
func (inliner *inliner) moveTo(side github.DiffSide, count int) {
	inliner.test.Helper()
	inliner.cursorTo(fmt.Sprintf("line %d on side %v", count, side), func(diffView *diffview.Model) bool {
		line, ok := diffView.Cursor()
		return ok && line.Side == side && line.Number() == count
	})
}

// toThread puts the diff cursor on the thread with the given ID.
func (inliner *inliner) toThread(identifier string) {
	inliner.test.Helper()
	inliner.cursorTo("thread "+identifier, func(diffView *diffview.Model) bool {
		threads := diffView.ThreadsAtCursor()
		return len(threads) == 1 && threads[0].Identifier == identifier
	})
}

// methods returns the recorded calls of the given fake methods.
func (inliner *inliner) methods(names ...string) []string {
	var out []string
	for _, call := range inliner.client.Calls() {
		for _, name := range names {
			if call.Method == name {
				out = append(out, name)
			}
		}
	}
	return out
}

func (inliner *inliner) pendingThread() github.ReviewThread {
	inliner.test.Helper()
	for _, thread := range inliner.view.threads {
		for _, comment := range thread.Comments {
			if comment.Pending {
				return thread
			}
		}
	}
	inliner.test.Fatal("no pending thread shown")
	return github.ReviewThread{}
}

func TestInlineCommentOnEachSide(test *testing.T) {
	tests := []struct {
		name string
		side github.DiffSide
		line int
		want github.AddThreadInput
	}{
		{"context line is on the right", github.SideRight, 10, github.AddThreadInput{Path: clientPath, Line: 10, Side: github.SideRight}},
		{"added line is on the right", github.SideRight, 14, github.AddThreadInput{Path: clientPath, Line: 14, Side: github.SideRight}},
		{"deleted line is on the left", github.SideLeft, 14, github.AddThreadInput{Path: clientPath, Line: 14, Side: github.SideLeft}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.body = "please rename\n"
			inliner.moveTo(testCase.side, testCase.line)
			inliner.press("c")
			if diff := cmp.Diff([]string{""}, inliner.openedOn); diff != "" {
				test.Fatalf("editor opened on (-want +got):\n%s", diff)
			}
			if _, asked := inliner.lastConfirm(); asked {
				test.Error("a pending comment must not ask for confirmation")
			}
			started := inliner.client.StartedReviews()
			if len(started) != 1 || started[0].CommitObjectIdentifier != headSHA {
				test.Fatalf("started = %+v", started)
			}
			got := inliner.client.AddedThreads()
			testCase.want.Repository, testCase.want.Number, testCase.want.Body = testRepository, 7, "please rename"
			testCase.want.PendingReviewIdentifier = started[0].ReviewIdentifier
			if diff := cmp.Diff([]github.AddThreadInput{testCase.want}, got); diff != "" {
				test.Errorf("added (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Pending comment added"}, inliner.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if len(inliner.view.drafts) != 0 {
				test.Errorf("draft kept: %v", inliner.view.drafts)
			}
			if thread := inliner.pendingThread(); thread.Line != testCase.line || thread.DiffSide != testCase.want.Side {
				test.Errorf("shown thread = %+v", thread)
			}
		})
	}
}

func TestInlineCommentsJoinTheSamePendingReview(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "one"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	inliner.body = "two"
	inliner.moveTo(github.SideRight, 11)
	inliner.press("c")
	started, added := inliner.client.StartedReviews(), inliner.client.AddedThreads()
	if len(started) != 1 || len(added) != 2 {
		test.Fatalf("started %d reviews, added %d threads", len(started), len(added))
	}
	if added[0].PendingReviewIdentifier != started[0].ReviewIdentifier || added[1].PendingReviewIdentifier != started[0].ReviewIdentifier {
		test.Errorf("threads went to %q and %q, want %q", added[0].PendingReviewIdentifier, added[1].PendingReviewIdentifier, started[0].ReviewIdentifier)
	}
	if status, _ := inliner.client.GetReviewStatus(test.Context(), testRepository, 7); status.Pending == nil || status.Pending.CommentCount != 2 {
		test.Errorf("pending = %+v", status.Pending)
	}
}

func TestInlineCommentJoinsAnExistingPendingReview(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "PRR_x", CommitObjectIdentifier: headSHA, CommentCount: 3})
	inliner.body = "more"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	if len(inliner.client.StartedReviews()) != 0 {
		test.Error("started a second review")
	}
	if got := inliner.client.AddedThreads(); len(got) != 1 || got[0].PendingReviewIdentifier != "PRR_x" {
		test.Errorf("added = %+v", got)
	}
}

func TestInlineMultiLineComment(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "this whole block"
	inliner.moveTo(github.SideRight, 14)
	inliner.press("V")
	if !inliner.view.CapturesKeys() {
		test.Fatal("visual mode does not capture keys")
	}
	inliner.typeText("jj")
	if out := ansi.Strip(inliner.render()); strings.Count(out, markSelectedForTest) != 2 {
		test.Errorf("want 2 selected rows marked plus the cursor row:\n%s", out)
	}
	inliner.typeText("c")
	if inliner.view.CapturesKeys() {
		test.Error("visual mode still active after c")
	}
	want := github.AddThreadInput{
		Repository: testRepository, Number: 7, Path: clientPath, Line: 16, Side: github.SideRight,
		StartLine: 14, StartSide: github.SideRight, Body: "this whole block",
		PendingReviewIdentifier: inliner.client.StartedReviews()[0].ReviewIdentifier,
	}
	if diff := cmp.Diff([]github.AddThreadInput{want}, inliner.client.AddedThreads()); diff != "" {
		test.Errorf("added (-want +got):\n%s", diff)
	}
	if thread := inliner.pendingThread(); thread.StartLine != 14 || thread.Line != 16 {
		test.Errorf("shown thread = %+v", thread)
	}
}

// markSelectedForTest is diffview's gutter mark of selected rows.
const markSelectedForTest = "▎"

func TestVisualRefusesTwoSidesAndEscLeaves(test *testing.T) {
	inliner := newInliner(test)
	inliner.moveTo(github.SideLeft, 15)
	inliner.press("V")
	inliner.messages = nil
	inliner.typeText("j") // the next line is an added one
	if diff := cmp.Diff([]string{diffview.ErrMixesSides.Error()}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	inliner.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if inliner.view.CapturesKeys() || inliner.view.filePane.VisualActive() {
		test.Error("esc did not leave visual mode")
	}
	if len(inliner.openedOn) != 0 {
		test.Error("esc opened the editor")
	}
}

func TestVisualOnHunkHeaderDoesNotStart(test *testing.T) {
	inliner := newInliner(test)
	inliner.diffView().GotoTop()
	inliner.messages = nil
	inliner.press("V")
	if inliner.view.CapturesKeys() {
		test.Error("visual mode started on a hunk header")
	}
	if diff := cmp.Diff([]string{"move onto a diff line to select lines"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
}

// TestVisualReleasesOnEscThroughRoot is the workspace's entry for the
// esc-releases-capture rule for visual mode, driven through the real root.
func TestVisualReleasesOnEscThroughRoot(test *testing.T) {
	inliner := newInliner(test)
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	model := userinterface.New(inliner.view, theme.New(true), userinterface.Options{Tick: noTick})
	step := func(message tea.Msg) { _, _ = model.Update(message) }
	step(tea.WindowSizeMsg{Width: 110, Height: 41})
	inliner.moveTo(github.SideRight, 10)
	step(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if !inliner.view.CapturesKeys() {
		test.Fatal("V through the root did not start visual mode")
	}
	step(tea.KeyPressMsg{Code: 'x', Text: "x"}) // keys are raw while captured: x is not a command
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if inliner.view.CapturesKeys() {
		test.Error("view still captures keys after esc")
	}
}

func TestInlineEmptyBodyIsAnError(test *testing.T) {
	for _, body := range []string{"", " \n\t"} {
		inliner := newInliner(test)
		inliner.body = body
		inliner.moveTo(github.SideRight, 10)
		inliner.press("c")
		if !strings.Contains(inliner.lastError(), "empty comment") {
			test.Errorf("body %q: error = %q", body, inliner.lastError())
		}
		if got := inliner.methods(fake.MethodStartReview, fake.MethodAddReviewThread); len(got) != 0 {
			test.Errorf("body %q: client called: %v", body, got)
		}
	}
}

func TestInlineFailedPostKeepsDraft(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetError(fake.MethodAddReviewThread, errors.New("boom"))
	inliner.body = "my draft"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	if !strings.Contains(inliner.lastError(), "boom") || !strings.Contains(inliner.lastError(), "draft") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if len(inliner.view.drafts) != 1 {
		test.Fatalf("drafts = %v", inliner.view.drafts)
	}
	inliner.client.SetError(fake.MethodAddReviewThread, nil)
	inliner.press("c")
	if diff := cmp.Diff([]string{"", "my draft"}, inliner.openedOn); diff != "" {
		test.Errorf("second editor (-want +got):\n%s", diff)
	}
	if len(inliner.client.AddedThreads()) != 1 || len(inliner.view.drafts) != 0 {
		test.Errorf("retry: added %d, drafts %v", len(inliner.client.AddedThreads()), inliner.view.drafts)
	}
	// A generic failure may have written the comment, so the first try's review stays and the retry joins it.
	if len(inliner.client.StartedReviews()) != 1 {
		test.Errorf("started %d reviews", len(inliner.client.StartedReviews()))
	}
}

func TestInlineEditorFailureKeepsTypedText(test *testing.T) {
	inliner := newInliner(test)
	inliner.body, inliner.err = "typed before the crash", errors.New("exit status 1")
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	if len(inliner.view.drafts) != 1 || len(inliner.client.AddedThreads()) != 0 {
		test.Errorf("drafts %v, added %d", inliner.view.drafts, len(inliner.client.AddedThreads()))
	}
}

func TestInlineRefusals(test *testing.T) {
	test.Run("diff not loaded", func(test *testing.T) {
		composer := newReviewer(test)
		composer.selectFile()
		composer.view.diff = DiffState{Err: errors.New("nope")} // as after a first load that failed
		composer.messages, composer.body = nil, "x"
		composer.press("c")
		if diff := cmp.Diff([]string{"diff not loaded"}, composer.infoMessages()); diff != "" || len(composer.openedOn) != 0 {
			test.Errorf("infos (-want +got):\n%s\nopened %v", diff, composer.openedOn)
		}
	})
	test.Run("head moved", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.client.SetHeadSHA(testRepository, 7, "bbbb2222")
		inliner.body = "x"
		inliner.moveTo(github.SideRight, 10)
		inliner.press("c")
		if !strings.Contains(inliner.lastError(), "head moved") {
			test.Errorf("error = %q", inliner.lastError())
		}
		if got := inliner.methods(fake.MethodStartReview, fake.MethodAddReviewThread); len(got) != 0 || len(inliner.view.drafts) != 1 {
			test.Errorf("calls %v, drafts %v", got, inliner.view.drafts)
		}
	})
	test.Run("pending review on another commit", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "PRR_old", CommitObjectIdentifier: "old1111"})
		inliner.body = "x"
		inliner.moveTo(github.SideRight, 10)
		inliner.press("c")
		if !strings.Contains(inliner.lastError(), "older commit") || len(inliner.client.AddedThreads()) != 0 {
			test.Errorf("error %q, added %d", inliner.lastError(), len(inliner.client.AddedThreads()))
		}
	})
	test.Run("hunk header", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.diffView().GotoTop()
		inliner.messages = nil
		inliner.press("c")
		if diff := cmp.Diff([]string{"move onto a diff line to comment on it"}, inliner.infoMessages()); diff != "" || len(inliner.openedOn) != 0 {
			test.Errorf("infos (-want +got):\n%s\nopened %v", diff, inliner.openedOn)
		}
	})
}

// addPending adds a comment on new line 10 and moves the cursor onto its thread.
func (inliner *inliner) addPending(body string) github.ReviewThread {
	inliner.test.Helper()
	inliner.body = body
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	thread := inliner.pendingThread()
	inliner.toThread(thread.Identifier)
	return thread
}

func TestEditPendingComment(test *testing.T) {
	inliner := newInliner(test)
	thread := inliner.addPending("first draft")
	inliner.openedOn, inliner.messages = nil, nil
	inliner.body = "second draft"
	inliner.press("e")
	if diff := cmp.Diff([]string{"first draft"}, inliner.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("editing a pending comment must not ask for confirmation")
	}
	if got := inliner.pendingThread().Comments[0].Body; got != "second draft" {
		test.Errorf("body = %q", got)
	}
	if diff := cmp.Diff([]string{"Comment updated"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if thread.Identifier != inliner.pendingThread().Identifier {
		test.Error("edit changed the thread")
	}
}

func TestEditPendingEmptyBodyAndUnchanged(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("keep me")
	inliner.body = ""
	inliner.press("e")
	if !strings.Contains(inliner.lastError(), "empty comment") {
		test.Errorf("error = %q", inliner.lastError())
	}
	inliner.client.SetError(fake.MethodUpdateReviewComment, errors.New("boom"))
	inliner.body = "changed"
	inliner.press("e")
	if len(inliner.view.drafts) != 1 || !strings.Contains(inliner.lastError(), "boom") {
		test.Errorf("failed edit: drafts %v, error %q", inliner.view.drafts, inliner.lastError())
	}
	inliner.client.SetError(fake.MethodUpdateReviewComment, nil)
	inliner.messages, inliner.body = nil, "keep me"
	inliner.press("e") // the kept draft replaces the body in the editor; the editor returns the original text
	if diff := cmp.Diff([]string{"comment unchanged"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
}

func TestEditAndDeleteOnlyYourComments(test *testing.T) {
	inliner := newInliner(test)
	inliner.press(")") // a thread by someone else
	inliner.messages = nil
	inliner.press("e")
	inliner.press("D")
	none := "no comment of yours here"
	if diff := cmp.Diff([]string{none, none}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
	if len(inliner.openedOn) != 0 {
		test.Error("editor opened on someone else's comment")
	}
	inliner.diffView().GotoTop() // a hunk header: no thread at all
	inliner.messages = nil
	inliner.press("e")
	if diff := cmp.Diff([]string{none}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos on a line without a thread (-want +got):\n%s", diff)
	}
}

func TestDeletePendingComment(test *testing.T) {
	inliner := newInliner(test)
	thread := inliner.addPending("oops")
	inliner.messages = nil
	inliner.press("D")
	confirmation, ok := inliner.lastConfirm()
	if !ok || confirmation.Text != "Delete pending comment on "+clientPath+":10?" {
		test.Fatalf("confirmation = %q (%v)", confirmation.Text, ok)
	}
	if len(inliner.client.DeletedComments()) != 0 {
		test.Fatal("deleted before the answer")
	}
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{thread.Comments[0].Identifier}, inliner.client.DeletedComments()); diff != "" {
		test.Errorf("deleted (-want +got):\n%s", diff)
	}
	for _, secondThread := range inliner.view.threads {
		if secondThread.Identifier == thread.Identifier {
			test.Error("deleted thread still shown")
		}
	}
	if diff := cmp.Diff([]string{"Comment deleted"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
}

func TestDeletePendingFailureIsReported(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("oops")
	inliner.client.SetError(fake.MethodDeleteReviewComment, errors.New("boom"))
	inliner.messages = nil
	inliner.press("D")
	confirmation, _ := inliner.lastConfirm()
	inliner.run(confirmation.OnYes)
	if !strings.Contains(inliner.lastError(), "boom") {
		test.Errorf("error = %q", inliner.lastError())
	}
}

func TestInlineKeysAreInHelp(test *testing.T) {
	harness := loaded(test, 100, 30)
	got := map[string]bool{}
	for _, bindings := range harness.view.FullHelp() {
		for _, binding := range bindings {
			if key := binding.Keys(); len(key) == 1 {
				got[key[0]] = true
			}
		}
	}
	for _, key := range []string{"V", "e", "D"} {
		if !got[key] {
			test.Errorf("%s is not in the help", key)
		}
	}
}

// submitElsewhere publishes the pending review on the server, as submitting
// it on github.com would, without the view hearing about it.
func (inliner *inliner) submitElsewhere() {
	inliner.test.Helper()
	in := github.SubmitReviewInput{
		Repository: testRepository, Number: 7, PullRequestIdentifier: pullRequestNodeIdentifier, Event: github.ReviewEventComment,
		PendingReviewIdentifier: inliner.client.StartedReviews()[0].ReviewIdentifier,
	}
	if err := inliner.client.SubmitReview(inliner.test.Context(), in); err != nil {
		inliner.test.Fatal(err)
	}
}

func (inliner *inliner) serverBody(identifier string) string {
	inliner.test.Helper()
	page, err := inliner.client.ListReviewThreads(inliner.test.Context(), testRepository, 7, github.PageRequest{})
	if err != nil {
		inliner.test.Fatal(err)
	}
	for _, thread := range page.Items {
		for _, comment := range thread.Comments {
			if comment.Identifier == identifier {
				return comment.Body
			}
		}
	}
	inliner.test.Fatalf("comment %s not on the server", identifier)
	return ""
}

// mineIdentifier is the ID of the thread built by mine.
const mineIdentifier = "t-mine"

// mine returns a published thread by the viewer on new line 10 of client.go.
func (inliner *inliner) mine(canUpdate, canDelete bool, comments ...github.ReviewComment) github.ReviewThread {
	if len(comments) == 0 {
		comments = []github.ReviewComment{{Identifier: "c-mine", Author: inliner.client.Viewer, Body: "published note"}}
	}
	for i := range comments {
		comments[i].Author = inliner.client.Viewer
		comments[i].ViewerCanUpdate, comments[i].ViewerCanDelete = canUpdate, canDelete
	}
	return github.ReviewThread{Identifier: mineIdentifier, Path: clientPath, Line: 10, DiffSide: github.SideRight, Comments: comments}
}

// seed sets the server's threads (the shared ones plus extra) and refreshes the view.
func (inliner *inliner) seed(extra ...github.ReviewThread) {
	inliner.test.Helper()
	inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), extra...))
	inliner.press("R")
}

func (inliner *inliner) updates() []string { return inliner.methods(fake.MethodUpdateReviewComment) }

func (inliner *inliner) deletes() []string { return inliner.methods(fake.MethodDeleteReviewComment) }

func TestEditPublishedCommentAsksAfterTheEditor(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true))
	inliner.toThread(mineIdentifier)
	inliner.messages, inliner.body = nil, "published, revised"
	inliner.press("e")
	if diff := cmp.Diff([]string{"published note"}, inliner.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	confirmation, ok := inliner.lastConfirm()
	if !ok || confirmation.Text != "Update your published comment on #7 "+clientPath+":10?" {
		test.Fatalf("confirmation = %q (%v)", confirmation.Text, ok)
	}
	if len(inliner.updates()) != 0 {
		test.Fatal("updated before the answer")
	}
	inliner.run(confirmation.OnYes)
	if got := inliner.serverBody("c-mine"); got != "published, revised" {
		test.Errorf("body = %q", got)
	}
	if diff := cmp.Diff([]string{"Comment updated"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
}

func TestDeletePublishedCommentAsks(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true))
	inliner.toThread(mineIdentifier)
	inliner.messages = nil
	inliner.press("D")
	confirmation, ok := inliner.lastConfirm()
	if !ok || confirmation.Text != "Delete your published comment on #7 "+clientPath+":10?" {
		test.Fatalf("confirmation = %q (%v)", confirmation.Text, ok)
	}
	if len(inliner.deletes()) != 0 {
		test.Fatal("deleted before the answer")
	}
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{"c-mine"}, inliner.client.DeletedComments()); diff != "" {
		test.Errorf("deleted (-want +got):\n%s", diff)
	}
}

func TestEditPublishedUnchangedWithATrailingNewline(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "published note\n"}))
	inliner.toThread(mineIdentifier)
	inliner.messages, inliner.body = nil, "published note\n" // saved as it was offered
	inliner.press("e")
	if _, asked := inliner.lastConfirm(); asked || len(inliner.updates()) != 0 {
		test.Errorf("asked %v, updates %v", asked, inliner.updates())
	}
	if diff := cmp.Diff([]string{"comment unchanged"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
}

// TestPendingTurnedPublishedWhileTheEditorIsOpen: fresh state decides, so a
// comment that was pending when e was pressed is never silently updated once
// it has been published.
func TestPendingTurnedPublishedWhileTheEditorIsOpen(test *testing.T) {
	inliner := newInliner(test)
	identifier := inliner.addPending("original").Comments[0].Identifier
	inliner.view.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			inliner.submitElsewhere() // submitted on github.com while the editor is open
			return done("rewritten", nil)
		}
	}
	inliner.messages = nil
	inliner.press("e")
	if len(inliner.updates()) != 0 {
		test.Fatal("a published comment was updated without asking")
	}
	confirmation, ok := inliner.lastConfirm()
	if !ok || confirmation.Text != "Update your published comment on #7 "+clientPath+":10?" {
		test.Fatalf("confirmation = %q (%v)", confirmation.Text, ok)
	}
	inliner.run(confirmation.OnYes)
	if got := inliner.serverBody(identifier); got != "rewritten" {
		test.Errorf("body = %q", got)
	}
}

func TestPendingTurnedPublishedBeforeDeleteAsksTheRightQuestion(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("original")
	inliner.submitElsewhere() // the view still shows it as pending
	inliner.messages = nil
	inliner.press("D")
	if confirmation, ok := inliner.lastConfirm(); !ok || !strings.HasPrefix(confirmation.Text, "Delete your published comment on #7 ") {
		test.Errorf("confirmation = %q (%v)", confirmation.Text, ok)
	}
}

// retitle changes the pull request on the server, so a reload can be told
// from no reload.
func (inliner *inliner) retitle() {
	inliner.test.Helper()
	detail := testDetail()
	detail.Identifier, detail.HeadSHA, detail.Title = pullRequestNodeIdentifier, headSHA, "retitled"
	inliner.client.SetPullRequest(detail)
}

func (inliner *inliner) threadIdentifiers() (identifiers []string) {
	for _, thread := range inliner.view.threads {
		identifiers = append(identifiers, thread.Identifier)
	}
	return identifiers
}

func TestGoneOrCannotChangeIsRefusedAndRefreshed(test *testing.T) {
	cases := []struct {
		name   string
		change func(testInliner *inliner, identifier string)
		want   string
		// check is what the refresh must have made the view show.
		check func(test *testing.T, testInliner *inliner)
	}{
		{"deleted elsewhere", func(inliner *inliner, identifier string) {
			if err := inliner.client.DeleteReviewComment(test.Context(), identifier); err != nil {
				test.Fatal(err)
			}
		}, "that comment is gone", func(test *testing.T, inliner *inliner) {
			if slices.Contains(inliner.threadIdentifiers(), mineIdentifier) {
				test.Error("the deleted comment's thread is still shown")
			}
		}},
		{"can no longer be changed", func(inliner *inliner, identifier string) {
			inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), inliner.mine(false, false, github.ReviewComment{Identifier: identifier, Body: "published note"})))
		}, "you can no longer change that comment", func(test *testing.T, inliner *inliner) {
			for _, thread := range inliner.view.threads {
				if thread.Identifier == mineIdentifier && thread.Comments[0].ViewerCanUpdate {
					test.Error("the view still shows the comment as changeable")
				}
			}
		}},
	}
	for _, testCase := range cases {
		prepare := func(test *testing.T) *inliner {
			inliner := newInliner(test)
			inliner.seed(inliner.mine(true, true))
			inliner.toThread(mineIdentifier)
			inliner.retitle()
			inliner.messages = nil
			return inliner
		}
		refreshed := func(test *testing.T, inliner *inliner) {
			test.Helper()
			if inliner.view.detail.Title != "retitled" {
				test.Error("the pull request was not reloaded")
			}
			testCase.check(test, inliner)
		}
		test.Run(testCase.name+"/edit", func(test *testing.T) {
			inliner := prepare(test)
			inliner.view.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
				return func() tea.Msg {
					testCase.change(inliner, "c-mine")
					return done("rewritten", nil)
				}
			}
			inliner.press("e")
			if !strings.Contains(inliner.lastError(), testCase.want) {
				test.Errorf("error = %q", inliner.lastError())
			}
			if _, asked := inliner.lastConfirm(); asked || len(inliner.updates()) != 0 {
				test.Errorf("asked %v, updates %v", asked, inliner.updates())
			}
			refreshed(test, inliner)
		})
		test.Run(testCase.name+"/delete", func(test *testing.T) {
			inliner := prepare(test)
			testCase.change(inliner, "c-mine")
			before := len(inliner.deletes())
			inliner.press("D")
			if !strings.Contains(inliner.lastError(), testCase.want) {
				test.Errorf("error = %q", inliner.lastError())
			}
			if _, asked := inliner.lastConfirm(); asked || len(inliner.deletes()) != before {
				test.Errorf("asked %v, deletes %v", asked, inliner.deletes())
			}
			refreshed(test, inliner)
		})
	}
}

func TestDeleteRechecksAfterTheAnswer(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("original")
	inliner.retitle()
	inliner.messages = nil
	inliner.press("D")
	confirmation, ok := inliner.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	// Someone deletes it while the question is open.
	if err := inliner.client.DeleteReviewComment(test.Context(), inliner.pendingThread().Comments[0].Identifier); err != nil {
		test.Fatal(err)
	}
	before := len(inliner.deletes())
	inliner.run(confirmation.OnYes)
	if len(inliner.deletes()) != before {
		test.Error("delete sent for a comment that is gone")
	}
	if !strings.Contains(inliner.lastError(), "that comment is gone") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if inliner.view.detail.Title != "retitled" {
		test.Error("the view was not refreshed")
	}
}

// A refused or failed edit never throws the text away.
func TestRefusedEditKeepsTheText(test *testing.T) {
	theirsFirst := func(inliner *inliner) github.ReviewThread {
		thread := inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "published note"})
		thread.Comments = append([]github.ReviewComment{{Identifier: "c-bob", Author: "bob", Body: "the question"}}, thread.Comments...)
		return thread
	}
	rewrite := func(inliner *inliner, during func()) {
		inliner.view.edit = func(initial string, done func(string, error) tea.Msg) tea.Cmd {
			inliner.openedOn = append(inliner.openedOn, initial)
			return func() tea.Msg {
				if during != nil {
					during()
					during = nil // only while the first editor is open
				}
				return done("my rewrite", nil)
			}
		}
	}
	setup := func(test *testing.T) *inliner {
		inliner := newInliner(test)
		inliner.seed(theirsFirst(inliner))
		inliner.toThread(mineIdentifier)
		inliner.messages = nil
		return inliner
	}
	replyKey := target{kind: kindReply, threadIdentifier: mineIdentifier}.draftKey()
	deleteMine := func(inliner *inliner) func() {
		return func() {
			if err := inliner.client.DeleteReviewComment(test.Context(), "c-mine"); err != nil {
				test.Fatal(err)
			}
		}
	}

	test.Run("gone before the re-read: the text becomes a reply draft", func(test *testing.T) {
		inliner := setup(test)
		rewrite(inliner, deleteMine(inliner))
		inliner.press("e")
		if want := "that comment is gone; refreshed; your text is kept as a reply draft"; inliner.lastError() != want {
			test.Errorf("error = %q, want %q", inliner.lastError(), want)
		}
		if diff := cmp.Diff(map[string]string{replyKey: "my rewrite"}, inliner.view.drafts); diff != "" {
			test.Errorf("drafts (-want +got):\n%s", diff)
		}
		// The thread survives (bob's comment), and r starts from the text.
		inliner.toThread(mineIdentifier)
		inliner.openedOn = nil
		inliner.press("r")
		if diff := cmp.Diff([]string{"my rewrite"}, inliner.openedOn); diff != "" {
			test.Errorf("reply editor opened on (-want +got):\n%s", diff)
		}
	})
	test.Run("gone at the write: the text becomes a reply draft", func(test *testing.T) {
		inliner := setup(test)
		rewrite(inliner, nil)
		// The re-read finds the comment; the write finds it gone.
		inliner.client.SetError(fake.MethodUpdateReviewComment, github.ErrNotFound)
		inliner.press("e")
		confirmation, ok := inliner.lastConfirm() // a published comment asks first
		if !ok {
			test.Fatal("no confirmation")
		}
		inliner.run(confirmation.OnYes)
		if !strings.Contains(inliner.lastError(), "your text is kept as a reply draft") {
			test.Errorf("error = %q", inliner.lastError())
		}
		if diff := cmp.Diff(map[string]string{replyKey: "my rewrite"}, inliner.view.drafts); diff != "" {
			test.Errorf("drafts (-want +got):\n%s", diff)
		}
	})
	test.Run("an existing reply draft is kept in front of it", func(test *testing.T) {
		inliner := setup(test)
		inliner.view.drafts[replyKey] = "half a reply"
		rewrite(inliner, deleteMine(inliner))
		inliner.press("e")
		if got := inliner.view.drafts[replyKey]; got != "half a reply\n\nmy rewrite" {
			test.Errorf("reply draft = %q", got)
		}
	})
	test.Run("cannot be changed any more: the text becomes a reply draft", func(test *testing.T) {
		inliner := setup(test)
		rewrite(inliner, func() {
			thread := inliner.mine(false, false, github.ReviewComment{Identifier: "c-mine", Body: "published note"})
			inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), thread))
		})
		inliner.press("e")
		if !strings.Contains(inliner.lastError(), "your text is kept as a reply draft") {
			test.Errorf("error = %q", inliner.lastError())
		}
		if diff := cmp.Diff(map[string]string{replyKey: "my rewrite"}, inliner.view.drafts); diff != "" {
			test.Errorf("drafts (-want +got):\n%s", diff)
		}
	})
	test.Run("after yes on a published comment", func(test *testing.T) {
		inliner := setup(test)
		rewrite(inliner, nil)
		inliner.press("e")
		confirmation, ok := inliner.lastConfirm()
		if !ok {
			test.Fatal("no confirmation")
		}
		deleteMine(inliner)()
		inliner.run(confirmation.OnYes)
		if diff := cmp.Diff(map[string]string{replyKey: "my rewrite"}, inliner.view.drafts); diff != "" {
			test.Errorf("drafts (-want +got):\n%s", diff)
		}
	})
}

func TestEditOpensOnTheFreshBody(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true))
	inliner.toThread(mineIdentifier)
	// The body was changed on github.com since the threads were loaded.
	thread := inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "edited on the web"})
	inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), thread))
	inliner.body = "edited on the web"
	inliner.press("e")
	if diff := cmp.Diff([]string{"edited on the web"}, inliner.openedOn); diff != "" {
		test.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	// And a comment that is already gone is refused before the editor.
	secondInliner := newInliner(test)
	secondInliner.seed(secondInliner.mine(true, true))
	secondInliner.toThread(mineIdentifier)
	if err := secondInliner.client.DeleteReviewComment(test.Context(), "c-mine"); err != nil {
		test.Fatal(err)
	}
	secondInliner.press("e")
	if len(secondInliner.openedOn) != 0 || !strings.Contains(secondInliner.lastError(), "that comment is gone") {
		test.Errorf("opened %v, error %q", secondInliner.openedOn, secondInliner.lastError())
	}
}

func TestVisualModeEndsWhenTheFileGoes(test *testing.T) {
	inliner := newInliner(test)
	inliner.moveTo(github.SideRight, 10)
	inliner.press("V")
	if !inliner.view.CapturesKeys() {
		test.Fatal("visual mode did not start")
	}
	// A reload no longer lists the file: the Overview is shown.
	detail := testDetail()
	detail.Identifier = pullRequestNodeIdentifier
	detail.Files = detail.Files[:0]
	detail.Files = append(detail.Files, github.ChangedFile{Path: "README.md", Additions: 2, Change: github.FileModified})
	inliner.send(pullRequestLoadedMessage{owner: inliner.view, repository: testRepository, number: 7, sequence: inliner.view.sequence, detail: detail})
	if inliner.view.selection() != 0 {
		test.Fatalf("the selection did not move to the Overview: sel = %d", inliner.view.selection())
	}
	if inliner.view.CapturesKeys() || inliner.view.filePane.VisualActive() {
		test.Errorf("visual mode survived onto the Overview: captures %v, active %v", inliner.view.CapturesKeys(), inliner.view.filePane.VisualActive())
	}
	inliner.openedOn = nil
	inliner.typeText("c")
	if len(inliner.openedOn) != 0 || len(inliner.client.AddedThreads()) != 0 {
		test.Errorf("c after the file went opened %v, added %d", inliner.openedOn, len(inliner.client.AddedThreads()))
	}
}

func TestVisualModeEndsWhenTheSelectionOrTheDiffChanges(test *testing.T) {
	test.Run("another file is selected", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.moveTo(github.SideRight, 10)
		inliner.press("V")
		inliner.view.selectEntry(inliner.view.selection() + 1)
		if inliner.view.filePane.VisualActive() {
			test.Error("the selection outlived its file")
		}
	})
	test.Run("the diff reloads", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.moveTo(github.SideRight, 10)
		inliner.press("V")
		inliner.press("R")
		if inliner.view.CapturesKeys() {
			test.Error("visual mode outlived a diff reload")
		}
	})
}

func TestInlineCommentKeepsTheHeadOfItsDiff(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "x"
	inliner.moveTo(github.SideRight, 10)
	// While the editor is open a newer diff is shown, and the server moved on
	// with it. The comment's lines come from the older diff.
	inliner.view.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			inliner.client.SetHeadSHA(testRepository, 7, "bbbb2222")
			inliner.view.diffHead = "bbbb2222"
			return done("x", nil)
		}
	}
	inliner.press("c")
	if !strings.Contains(inliner.lastError(), "head moved") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if got := inliner.methods(fake.MethodStartReview, fake.MethodAddReviewThread); len(got) != 0 {
		test.Errorf("client called: %v", got)
	}
}

func TestPendingReviewOnAnUnknownCommitIsRefused(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "PRR_x"}) // no commit
	inliner.body = "x"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	if !strings.Contains(inliner.lastError(), "unknown commit") || len(inliner.client.AddedThreads()) != 0 {
		test.Errorf("error %q, added %d", inliner.lastError(), len(inliner.client.AddedThreads()))
	}
}

func TestInlineDraftsAreKeyedBySide(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetError(fake.MethodAddReviewThread, errors.New("boom"))
	inliner.body = "for the old line"
	inliner.moveTo(github.SideLeft, 14)
	inliner.press("c")
	inliner.openedOn = nil
	inliner.moveTo(github.SideRight, 14) // the same number on the other side
	inliner.press("c")
	if diff := cmp.Diff([]string{""}, inliner.openedOn); diff != "" {
		test.Errorf("the draft of the deleted line was offered for the added one (-want +got):\n%s", diff)
	}
	inliner.openedOn = nil
	inliner.moveTo(github.SideLeft, 14)
	inliner.press("c")
	if diff := cmp.Diff([]string{"for the old line"}, inliner.openedOn); diff != "" {
		test.Errorf("the draft was not kept for its own line (-want +got):\n%s", diff)
	}
}

func TestMultiLineCommentOnTheOldSide(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "these two"
	inliner.moveTo(github.SideLeft, 14)
	inliner.press("V")
	inliner.typeText("j")
	inliner.typeText("c")
	want := github.AddThreadInput{
		Repository: testRepository, Number: 7, Path: clientPath, Line: 15, Side: github.SideLeft,
		StartLine: 14, StartSide: github.SideLeft, Body: "these two",
		PendingReviewIdentifier: inliner.client.StartedReviews()[0].ReviewIdentifier,
	}
	if diff := cmp.Diff([]github.AddThreadInput{want}, inliner.client.AddedThreads()); diff != "" {
		test.Errorf("added (-want +got):\n%s", diff)
	}
}

// A comment being saved holds back reviews and merges, as a review being
// submitted holds back comments.
func TestCommentWriteInFlightBlocksReviewsAndMerges(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.writes = 1
	for _, name := range []string{"approve", "request-changes", "review", "merge"} {
		inliner.messages = nil
		inliner.command(name)
		if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
			test.Errorf(":%s (-want +got):\n%s", name, diff)
		}
	}
	inliner.messages, inliner.body = nil, "x"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c")
	if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" || len(inliner.openedOn) != 0 {
		test.Errorf("c (-want +got):\n%s", diff)
	}
	inliner.view.writes = 0
	inliner.view.review.submitting = true
	inliner.messages = nil
	inliner.press("c")
	if diff := cmp.Diff([]string{"a review is being submitted"}, inliner.infoMessages()); diff != "" {
		test.Errorf("c while submitting (-want +got):\n%s", diff)
	}
}

func TestCommentWriteFlagClears(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "x"
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c") // the harness delivers the result
	if inliner.view.writes > 0 {
		test.Error("the flag stayed set after the write finished")
	}
	inliner.client.SetError(fake.MethodAddReviewThread, errors.New("boom"))
	inliner.moveTo(github.SideRight, 11)
	inliner.press("c")
	if inliner.view.writes > 0 {
		test.Error("the flag stayed set after a failed write")
	}
	inliner.toThread(inliner.pendingThread().Identifier)
	inliner.messages = nil
	inliner.press("D")
	if _, asked := inliner.lastConfirm(); !asked || (inliner.view.writes > 0) {
		test.Errorf("asked %v, flag %v: the flag must clear when the question is shown", asked, (inliner.view.writes > 0))
	}
}

func TestCommandsBehaveLikeTheKeys(test *testing.T) {
	test.Run("edit pending", func(test *testing.T) {
		inliner := newInliner(test)
		identifier := inliner.addPending("first draft").Comments[0].Identifier
		inliner.openedOn, inliner.messages, inliner.body = nil, nil, "second draft"
		inliner.command("edit-comment")
		if diff := cmp.Diff([]string{"first draft"}, inliner.openedOn); diff != "" {
			test.Errorf("editor opened on (-want +got):\n%s", diff)
		}
		if _, asked := inliner.lastConfirm(); asked {
			test.Error("a pending edit asked for confirmation")
		}
		if got := inliner.serverBody(identifier); got != "second draft" {
			test.Errorf("body = %q", got)
		}
	})
	test.Run("edit published", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(inliner.mine(true, true))
		inliner.toThread(mineIdentifier)
		inliner.messages, inliner.body = nil, "revised"
		inliner.command("edit-comment")
		if confirmation, ok := inliner.lastConfirm(); !ok || !strings.HasPrefix(confirmation.Text, "Update your published comment") {
			test.Errorf("confirmation = %q (%v)", confirmation.Text, ok)
		}
	})
	test.Run("delete", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("x")
		inliner.messages = nil
		inliner.command("delete-comment")
		if confirmation, ok := inliner.lastConfirm(); !ok || !strings.HasPrefix(confirmation.Text, "Delete pending comment on ") {
			test.Errorf("confirmation = %q (%v)", confirmation.Text, ok)
		}
	})
	test.Run("in the overview", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.press("h", "g g") // Files pane, Overview
		inliner.messages = nil
		inliner.command("edit-comment")
		if diff := cmp.Diff([]string{"no comment of yours here"}, inliner.infoMessages()); diff != "" {
			test.Errorf("infos (-want +got):\n%s", diff)
		}
	})
}

func TestEditAndDeleteRefusedWhileSubmitting(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("original")
	inliner.openedOn, inliner.messages = nil, nil
	inliner.view.review.submitting = true
	inliner.body = "x"
	inliner.press("e")
	inliner.press("D")
	if len(inliner.openedOn) != 0 {
		test.Error("editor opened while a review is being submitted")
	}
	busy := "a review is being submitted"
	if diff := cmp.Diff([]string{busy, busy}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("asked to delete while a review is being submitted")
	}
	if got := inliner.methods(fake.MethodUpdateReviewComment, fake.MethodDeleteReviewComment); len(got) != 0 {
		test.Errorf("mutations sent: %v", got)
	}
}

// Write access makes viewerCanUpdate/viewerCanDelete true on other people's
// comments (the fake models this), so authorship has to be checked too.
func TestOtherPeoplesCommentsAreNeverTargeted(test *testing.T) {
	theirs := func(identifier string) github.ReviewComment {
		return github.ReviewComment{Identifier: identifier, Author: "bob", Body: "theirs"}
	}
	test.Run("only their comment in the thread", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(github.ReviewThread{Identifier: mineIdentifier, Path: clientPath, Line: 10, DiffSide: github.SideRight, Comments: []github.ReviewComment{theirs("c-bob")}})
		inliner.toThread(mineIdentifier)
		inliner.messages = nil
		inliner.press("e")
		inliner.press("D")
		none := "no comment of yours here"
		if diff := cmp.Diff([]string{none, none}, inliner.infoMessages()); diff != "" {
			test.Errorf("infos (-want +got):\n%s", diff)
		}
		if _, asked := inliner.lastConfirm(); asked || len(inliner.openedOn) != 0 {
			test.Errorf("asked %v, editor opened on %v", asked, inliner.openedOn)
		}
	})
	test.Run("their reply is the last comment", func(test *testing.T) {
		inliner := newInliner(test)
		thread := inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "mine"})
		thread.Comments = append(thread.Comments, theirs("c-bob-reply"))
		inliner.seed(thread)
		inliner.toThread(mineIdentifier)
		inliner.messages = nil
		inliner.press("D")
		confirmation, ok := inliner.lastConfirm()
		if !ok {
			test.Fatal("no confirmation")
		}
		inliner.run(confirmation.OnYes)
		if diff := cmp.Diff([]string{"c-mine"}, inliner.client.DeletedComments()); diff != "" {
			test.Errorf("deleted (-want +got):\n%s", diff)
		}
		inliner.seed(thread)
		inliner.toThread(mineIdentifier)
		inliner.body = "mine, edited"
		inliner.press("e")
		if diff := cmp.Diff([]string{"mine"}, inliner.openedOn); diff != "" {
			test.Errorf("editor opened on (-want +got):\n%s", diff)
		}
	})
	test.Run("the fresh read shows it is not yours", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(inliner.mine(true, true))
		inliner.toThread(mineIdentifier)
		// On the server the comment is somebody else's now.
		other := github.ReviewThread{Identifier: mineIdentifier, Path: clientPath, Line: 10, DiffSide: github.SideRight, Comments: []github.ReviewComment{theirs("c-mine")}}
		inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), other))
		inliner.messages, inliner.body = nil, "rewritten"
		inliner.press("D")
		if !strings.Contains(inliner.lastError(), "not yours") {
			test.Errorf("delete error = %q", inliner.lastError())
		}
		if _, asked := inliner.lastConfirm(); asked {
			test.Error("asked to delete somebody else's comment")
		}
		// The view refreshed, so it no longer offers the comment at all; make
		// it stale again for the edit path.
		inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), inliner.mine(true, true)))
		inliner.press("R")
		inliner.toThread(mineIdentifier)
		inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), other))
		inliner.messages = nil
		inliner.press("e")
		if !strings.Contains(inliner.lastError(), "not yours") || len(inliner.updates()) != 0 {
			test.Errorf("edit error = %q, updates %v", inliner.lastError(), inliner.updates())
		}
	})
}

func TestDeleteMustNotDeleteAPublishedCommentThatWasAskedAboutAsPending(test *testing.T) {
	inliner := newInliner(test)
	identifier := inliner.addPending("original").Comments[0].Identifier
	inliner.messages = nil
	inliner.press("D")
	confirmation, ok := inliner.lastConfirm()
	if !ok || !strings.HasPrefix(confirmation.Text, "Delete pending comment on ") {
		test.Fatalf("confirmation = %q (%v)", confirmation.Text, ok)
	}
	inliner.submitElsewhere() // published while the question is open
	before := len(inliner.deletes())
	inliner.run(confirmation.OnYes)
	if len(inliner.deletes()) != before {
		test.Fatal("a published comment was deleted on the answer to the pending question")
	}
	again, ok := inliner.lastConfirm()
	if !ok || again.Text != "Delete your published comment on #7 "+clientPath+":10?" {
		test.Fatalf("second confirmation = %q (%v)", again.Text, ok)
	}
	inliner.run(again.OnYes)
	if diff := cmp.Diff([]string{identifier}, inliner.client.DeletedComments()); diff != "" {
		test.Errorf("deleted (-want +got):\n%s", diff)
	}
}

func TestPublishedEditPromptIsDroppedWhenTheViewChanged(test *testing.T) {
	inliner := newInliner(test)
	other := New(inliner.client, testRepository, 12)
	message := commentCheckedMessage{origin: origin{other, testRepository, 12}, question: "Update your published comment on #12 a.go:1?"}
	if _, command := inliner.view.Update(message); command != nil {
		test.Errorf("a prompt for another view produced %v", messages(command))
	}
}

func TestPickComment(test *testing.T) {
	mine := func(identifier string, pending bool) github.ReviewComment {
		return github.ReviewComment{Identifier: identifier, Pending: pending, ViewerDidAuthor: true, ViewerCanUpdate: true, ViewerCanDelete: true}
	}
	// A maintainer's flags are true on other people's comments.
	theirs := github.ReviewComment{Identifier: "theirs", ViewerCanUpdate: true, ViewerCanDelete: true}
	noFlags := github.ReviewComment{Identifier: "no-flags", ViewerDidAuthor: true}
	tests := []struct {
		name     string
		comments []github.ReviewComment
		change   change
		want     string // "" for none
	}{
		{"empty thread", nil, changeEdit, ""},
		{"the only comment", []github.ReviewComment{mine("a", false)}, changeEdit, "a"},
		{"the last one", []github.ReviewComment{mine("a", false), mine("b", false)}, changeEdit, "b"},
		{"pending beats a later published one", []github.ReviewComment{mine("a", false), mine("b", true), mine("c", false)}, changeEdit, "b"},
		{"the last pending one", []github.ReviewComment{mine("a", true), mine("b", true), mine("c", false)}, changeDelete, "b"},
		{"another author's last comment is skipped", []github.ReviewComment{mine("a", false), theirs}, changeDelete, "a"},
		{"only another author's", []github.ReviewComment{theirs}, changeEdit, ""},
		{"flags false", []github.ReviewComment{noFlags}, changeEdit, ""},
		{"flags false is skipped", []github.ReviewComment{mine("a", false), noFlags}, changeDelete, "a"},
		{"delete needs the delete flag", []github.ReviewComment{{Identifier: "a", ViewerDidAuthor: true, ViewerCanUpdate: true}}, changeDelete, ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, ok := pickComment(github.ReviewThread{Comments: testCase.comments}, testCase.change)
			if (testCase.want != "") != ok || got.Identifier != testCase.want {
				test.Errorf("picked %q (%v), want %q", got.Identifier, ok, testCase.want)
			}
		})
	}
}

// e and D each ask for their own permission: a comment that may be updated but
// not deleted (and the reverse) is offered by one key only.
func TestEditAndDeleteUseTheirOwnPermission(test *testing.T) {
	tests := []struct {
		name                 string
		canUpdate, canDelete bool
		key                  string
		offered              bool
	}{
		{"update only: e edits", true, false, "e", true},
		{"update only: D finds nothing", true, false, "D", false},
		{"delete only: D asks", false, true, "D", true},
		{"delete only: e finds nothing", false, true, "e", false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seed(inliner.mine(testCase.canUpdate, testCase.canDelete))
			inliner.toThread(mineIdentifier)
			inliner.messages, inliner.openedOn, inliner.body = nil, nil, "x"
			inliner.press(testCase.key)
			_, asked := inliner.lastConfirm()
			offered := len(inliner.openedOn) > 0 || asked
			if offered != testCase.offered {
				test.Errorf("offered = %v (editor %v, asked %v), want %v; infos %v", offered, inliner.openedOn, asked, testCase.offered, inliner.infoMessages())
			}
			if !testCase.offered {
				if diff := cmp.Diff([]string{"no comment of yours here"}, inliner.infoMessages()); diff != "" {
					test.Errorf("info (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// agree answers a question as the root does, but leaves the write it starts
// unrun, so the write is in flight until the returned command is run.
func (inliner *inliner) agree(confirmation userinterface.ConfirmMessage) tea.Cmd {
	inliner.test.Helper()
	_, command := inliner.view.Update(confirmation.OnYes())
	return command
}

func TestWriteAfterYesIsInFlightUntilItsResult(test *testing.T) {
	blocked := func(inliner *inliner) {
		test.Helper()
		for _, name := range []string{"approve", "request-changes", "review", "merge"} {
			inliner.messages = nil
			inliner.command(name)
			if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
				test.Errorf(":%s while the write is in flight (-want +got):\n%s", name, diff)
			}
		}
	}
	test.Run("delete", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("x")
		inliner.messages = nil
		inliner.press("D")
		confirmation, _ := inliner.lastConfirm()
		if inliner.view.writes > 0 {
			test.Fatal("flag set while the question is open")
		}
		write := inliner.agree(confirmation)
		if inliner.view.writes <= 0 {
			test.Fatal("flag not set after the yes")
		}
		blocked(inliner)
		inliner.run(write)
		if (inliner.view.writes > 0) || len(inliner.client.DeletedComments()) != 1 {
			test.Errorf("flag %v, deleted %v", (inliner.view.writes > 0), inliner.client.DeletedComments())
		}
	})
	test.Run("published edit", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(inliner.mine(true, true))
		inliner.toThread(mineIdentifier)
		inliner.messages, inliner.body = nil, "revised"
		inliner.press("e")
		confirmation, _ := inliner.lastConfirm()
		write := inliner.agree(confirmation)
		blocked(inliner)
		inliner.run(write)
		if inliner.view.writes > 0 {
			test.Error("flag still set after the result")
		}
	})
	test.Run("a failed write clears it too", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("x")
		inliner.client.SetError(fake.MethodDeleteReviewComment, errors.New("boom"))
		inliner.messages = nil
		inliner.press("D")
		confirmation, _ := inliner.lastConfirm()
		inliner.run(inliner.agree(confirmation))
		if inliner.view.writes > 0 {
			test.Error("flag still set after a failed write")
		}
	})
	test.Run("a stale refusal after the yes clears it", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("x")
		inliner.messages = nil
		inliner.press("D")
		confirmation, _ := inliner.lastConfirm()
		if err := inliner.client.DeleteReviewComment(test.Context(), inliner.pendingThread().Comments[0].Identifier); err != nil {
			test.Fatal(err)
		}
		inliner.run(inliner.agree(confirmation))
		if inliner.view.writes > 0 {
			test.Error("flag still set after a stale refusal")
		}
	})
	test.Run("reply", func(test *testing.T) {
		composer := newComposer(test)
		composer.onThread()
		composer.body = "thanks"
		composer.press("r")
		confirmation, ok := composer.lastConfirm()
		if !ok {
			test.Fatal("no confirmation")
		}
		_, write := composer.view.Update(confirmation.OnYes())
		if composer.view.writes <= 0 {
			test.Fatal("flag not set for a reply")
		}
		composer.run(write)
		if composer.view.writes > 0 {
			test.Error("flag still set after the reply was posted")
		}
	})
	test.Run("top-level comment", func(test *testing.T) {
		composer := newComposer(test)
		composer.body = "LGTM"
		composer.press("c")
		confirmation, ok := composer.lastConfirm()
		if !ok {
			test.Fatal("no confirmation")
		}
		_, write := composer.view.Update(confirmation.OnYes())
		if composer.view.writes <= 0 {
			test.Fatal("flag not set for a top-level comment")
		}
		composer.run(write)
		if composer.view.writes > 0 {
			test.Error("flag still set after the comment was posted")
		}
	})
}

func TestYesIsRefusedWhileAReviewOrMergeRuns(test *testing.T) {
	for name, busy := range map[string]func(testInliner *inliner){
		"review": func(inliner *inliner) { inliner.view.review.submitting = true },
		"merge":  func(inliner *inliner) { inliner.view.merge.merging = true },
	} {
		test.Run(name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.addPending("x")
			inliner.messages = nil
			inliner.press("D")
			confirmation, _ := inliner.lastConfirm()
			busy(inliner)
			before := len(inliner.deletes())
			inliner.run(confirmation.OnYes)
			if len(inliner.deletes()) != before || (inliner.view.writes > 0) {
				test.Errorf("deletes %v, flag %v", inliner.deletes(), (inliner.view.writes > 0))
			}
			if len(inliner.infoMessages()) == 0 {
				test.Error("no message for the refused yes")
			}
		})
	}
}

func TestAgreedForAnotherViewIsIgnored(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.writes = 1
	other := New(inliner.client, testRepository, 12)
	_, command := inliner.view.Update(commentAgreedMessage{origin: other.origin(), run: func() tea.Msg { return nil }})
	if command != nil || inliner.view.writes <= 0 {
		test.Errorf("cmd %v, flag %v: another view's yes must change nothing", command != nil, (inliner.view.writes > 0))
	}
	// A result for another view leaves the flag alone too.
	inliner.send(commentDeletedMessage{origin: other.origin()})
	inliner.send(postedMessage{origin: other.origin()})
	if inliner.view.writes <= 0 {
		test.Error("another view's result cleared the flag")
	}
}

// lineDraftKey is the draft `c` uses for a single line of client.go.
func lineDraftKey(side github.DiffSide, line int) string {
	return target{kind: kindInline, inline: &github.AddThreadInput{Path: clientPath, Side: side, Line: line}}.draftKey()
}

// A refused edit's text has one reachable home: the reply draft while its
// thread exists, else the draft of a new comment on its line.
func TestRefusedEditTextGoesToTheThreadOrTheLine(test *testing.T) {
	replyKey := target{kind: kindReply, threadIdentifier: mineIdentifier}.draftKey()
	lineKey := lineDraftKey(github.SideRight, 10)
	topKey := target{}.draftKey()
	twoComments := func(inliner *inliner) github.ReviewThread {
		thread := inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "published note"})
		thread.Comments = append([]github.ReviewComment{{Identifier: "c-bob", Author: "bob", Body: "the question"}}, thread.Comments...)
		return thread
	}
	single := func(inliner *inliner) github.ReviewThread { return inliner.mine(true, true) }
	outdated := func(inliner *inliner) github.ReviewThread { // no current line: shown below the diff
		thread := inliner.mine(true, true)
		thread.Line, thread.OriginalLine, thread.IsOutdated = 0, 10, true
		return thread
	}
	offDiff := func(inliner *inliner) github.ReviewThread { // a line the diff does not show
		thread := inliner.mine(true, true)
		thread.Line = 900
		return thread
	}
	deleteIt := func(inliner *inliner) {
		if err := inliner.client.DeleteReviewComment(test.Context(), "c-mine"); err != nil {
			test.Fatal(err)
		}
	}
	lock := func(inliner *inliner) {
		inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), inliner.mine(false, false, github.ReviewComment{Identifier: "c-mine", Body: "published note"})))
	}
	notYours := func(inliner *inliner) {
		thread := inliner.mine(true, true, github.ReviewComment{Identifier: "c-mine", Body: "published note"})
		thread.Comments[0].Author = "bob"
		inliner.client.SetReviewThreads(testRepository, 7, append(testThreads(), thread))
	}
	tests := []struct {
		name    string
		thread  func(testInliner *inliner) github.ReviewThread
		refuse  func(testInliner *inliner)
		wantKey string
		suffix  string
	}{
		{"gone, other comments remain", twoComments, deleteIt, replyKey, "your text is kept as a reply draft"},
		{"gone, it was the only comment", single, deleteIt, lineKey, "your text is kept as a draft for a new comment on " + clientPath + ":10"},
		{"gone, outdated thread without a line", outdated, deleteIt, topKey, "your text is kept as a draft for a top-level comment"},
		{"gone, line not in the diff", offDiff, deleteIt, topKey, "your text is kept as a draft for a top-level comment"},
		{"outdated thread that survives", func(inliner *inliner) github.ReviewThread {
			thread := outdated(inliner)
			thread.Comments = append([]github.ReviewComment{{Identifier: "c-bob", Author: "bob", Body: "the question"}}, thread.Comments...)
			return thread
		}, deleteIt, replyKey, "your text is kept as a reply draft"},
		{"locked, only comment", single, lock, replyKey, "your text is kept as a reply draft"},
		{"locked, other comments remain", twoComments, lock, replyKey, "your text is kept as a reply draft"},
		{"not yours any more", single, notYours, replyKey, "your text is kept as a reply draft"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.seed(testCase.thread(inliner))
			inliner.toThread(mineIdentifier)
			inliner.messages = nil
			first := true
			inliner.view.edit = func(initial string, done func(string, error) tea.Msg) tea.Cmd {
				inliner.openedOn = append(inliner.openedOn, initial)
				return func() tea.Msg {
					if first {
						first = false
						testCase.refuse(inliner) // while the editor is open
					}
					return done("my rewrite", nil)
				}
			}
			inliner.press("e")
			if !strings.Contains(inliner.lastError(), testCase.suffix) {
				test.Errorf("error = %q, want it to end with %q", inliner.lastError(), testCase.suffix)
			}
			if diff := cmp.Diff(map[string]string{testCase.wantKey: "my rewrite"}, inliner.view.drafts); diff != "" {
				test.Errorf("drafts (-want +got):\n%s", diff)
			}
			if testCase.wantKey == topKey {
				// c in the Overview starts from the text.
				inliner.press("h", "g g")
				inliner.openedOn = nil
				inliner.press("c")
				if diff := cmp.Diff([]string{"my rewrite"}, inliner.openedOn); diff != "" {
					test.Errorf("c in the Overview opened on (-want +got):\n%s", diff)
				}
			}
			if testCase.wantKey == lineKey {
				// The thread is gone after the refresh; c on its line starts from the text.
				if slices.Contains(inliner.threadIdentifiers(), mineIdentifier) {
					test.Fatal("the thread should be gone")
				}
				inliner.openedOn = nil
				inliner.moveTo(github.SideRight, 10)
				inliner.press("c")
				if diff := cmp.Diff([]string{"my rewrite"}, inliner.openedOn); diff != "" {
					test.Errorf("c on the line opened on (-want +got):\n%s", diff)
				}
			}
		})
	}
	test.Run("the write finds it gone", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(single(inliner))
		inliner.toThread(mineIdentifier)
		inliner.body = "my rewrite"
		inliner.client.SetError(fake.MethodUpdateReviewComment, github.ErrNotFound)
		inliner.press("e")
		confirmation, ok := inliner.lastConfirm()
		if !ok {
			test.Fatal("no confirmation")
		}
		// The comment is really gone with its thread by the time the thread list is read.
		deleteIt(inliner)
		inliner.run(confirmation.OnYes)
		if diff := cmp.Diff(map[string]string{lineKey: "my rewrite"}, inliner.view.drafts); diff != "" {
			test.Errorf("drafts (-want +got):\n%s", diff)
		}
	})
	test.Run("the thread list cannot be read: the line draft is always reachable", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(twoComments(inliner))
		inliner.toThread(mineIdentifier)
		inliner.body = "my rewrite"
		inliner.press("e")
		confirmation, _ := inliner.lastConfirm()
		deleteIt(inliner)
		inliner.client.SetError(fake.MethodListReviewThreads, errors.New("boom"))
		inliner.run(confirmation.OnYes)
		if _, ok := inliner.view.drafts[lineKey]; !ok {
			test.Errorf("drafts = %v, want the line draft", inliner.view.drafts)
		}
	})
	test.Run("joined with an existing draft once", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.seed(twoComments(inliner))
		inliner.toThread(mineIdentifier)
		inliner.view.drafts[replyKey] = "half a reply"
		inliner.body = "my rewrite"
		inliner.press("e")
		confirmation, _ := inliner.lastConfirm()
		deleteIt(inliner)
		inliner.run(confirmation.OnYes)
		if got := inliner.view.drafts[replyKey]; got != "half a reply\n\nmy rewrite" {
			test.Errorf("reply draft = %q", got)
		}
		// The same text again is not appended a second time.
		inliner.view.drafts[replyKey] = "my rewrite"
		inliner.send(postedMessage{origin: inliner.view.origin(), target: target{kind: kindEdit, commentIdentifier: "c-mine", threadIdentifier: mineIdentifier}, body: "my rewrite", err: errCommentGone, threadLive: true})
		if got := inliner.view.drafts[replyKey]; got != "my rewrite" {
			test.Errorf("a draft that already is the text was doubled: %q", got)
		}
	})
}

// The re-read of e lands after another editor was started: it must not open a
// second one over it.
func TestSupersededReadDoesNotOpenAnEditor(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true))
	inliner.toThread(mineIdentifier)
	// e: the read is in flight (held).
	_, read := inliner.view.Update(keys.Message{Sequence: "e"})
	if read == nil {
		test.Fatal("e started no read")
	}
	// Meanwhile c starts an inline editor on line 11.
	inliner.body = "inline text"
	inliner.moveTo(github.SideRight, 11)
	inliner.press("c")
	opened := len(inliner.openedOn)
	// The read lands now.
	inliner.run(read)
	if len(inliner.openedOn) != opened {
		test.Errorf("the late read opened an editor on %v", inliner.openedOn[opened:])
	}
	if len(inliner.client.AddedThreads()) != 1 {
		test.Errorf("the inline comment was lost: added %d", len(inliner.client.AddedThreads()))
	}
	// And pressing e twice opens one editor, on the second read.
	secondInliner := newInliner(test)
	secondInliner.seed(secondInliner.mine(true, true))
	secondInliner.toThread(mineIdentifier)
	_, first := secondInliner.view.Update(keys.Message{Sequence: "e"})
	_, second := secondInliner.view.Update(keys.Message{Sequence: "e"})
	secondInliner.openedOn, secondInliner.body = nil, "published note"
	secondInliner.run(first)
	if len(secondInliner.openedOn) != 0 {
		test.Errorf("the first read opened an editor: %v", secondInliner.openedOn)
	}
	secondInliner.run(second)
	if len(secondInliner.openedOn) != 1 {
		test.Errorf("editors opened = %d, want 1", len(secondInliner.openedOn))
	}
}

func TestReadIsRefusedWhenAReviewStartedMeanwhile(test *testing.T) {
	inliner := newInliner(test)
	inliner.seed(inliner.mine(true, true))
	inliner.toThread(mineIdentifier)
	_, read := inliner.view.Update(keys.Message{Sequence: "e"})
	inliner.view.review.submitting = true
	inliner.messages, inliner.openedOn = nil, nil
	inliner.run(read)
	if len(inliner.openedOn) != 0 {
		test.Error("an editor opened while a review is being submitted")
	}
	if diff := cmp.Diff([]string{"a review is being submitted"}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
}

// Only one comment write at a time, and each start is counted.
func TestEachWriteIsCountedAndExcludesTheNext(test *testing.T) {
	assertBlocked := func(test *testing.T, inliner *inliner, what string) {
		test.Helper()
		if inliner.view.writes != 1 {
			test.Fatalf("%s: writes = %d, want 1", what, inliner.view.writes)
		}
		for _, name := range []string{"approve", "request-changes", "review", "merge"} {
			inliner.messages = nil
			inliner.command(name)
			if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
				test.Errorf("%s: :%s (-want +got):\n%s", what, name, diff)
			}
		}
		inliner.messages, inliner.openedOn = nil, nil
		inliner.moveTo(github.SideRight, 12)
		inliner.press("c")
		inliner.press("r")
		inliner.press("e")
		inliner.press("D")
		if len(inliner.openedOn) != 0 {
			test.Errorf("%s: a second write opened an editor on %v", what, inliner.openedOn)
		}
		busy := commentSavingText
		if diff := cmp.Diff([]string{busy, busy, busy, busy}, inliner.infoMessages()); diff != "" {
			test.Errorf("%s: second writes (-want +got):\n%s", what, diff)
		}
	}
	test.Run("inline add", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.body = "x"
		inliner.moveTo(github.SideRight, 10)
		_, compose := inliner.view.Update(keys.Message{Sequence: "c"})
		_, write := inliner.view.Update(compose()) // the editor closed: the add starts, unrun
		assertBlocked(test, inliner, "inline add")
		inliner.run(write)
		if inliner.view.writes != 0 {
			test.Errorf("writes = %d after the result", inliner.view.writes)
		}
	})
	test.Run("pending edit", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("first")
		inliner.body = "second"
		_, read := inliner.view.Update(keys.Message{Sequence: "e"})
		_, compose := inliner.view.Update(read())
		_, write := inliner.view.Update(compose())
		assertBlocked(test, inliner, "pending edit")
		inliner.run(write)
		if inliner.view.writes != 0 {
			test.Errorf("writes = %d after the result", inliner.view.writes)
		}
	})
	test.Run("delete before its question", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("x")
		_, write := inliner.view.Update(keys.Message{Sequence: "D"})
		assertBlocked(test, inliner, "delete")
		inliner.run(write)
		if inliner.view.writes != 0 {
			test.Errorf("writes = %d after the question", inliner.view.writes)
		}
	})
}

func TestReviewAndMergeRefuseAtTheirYesWhileAWriteRuns(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.writes = 1
	before := len(inliner.client.Submissions())
	inliner.send(reviewSendMessage{flow: &inliner.view.review, repository: testRepository, number: 7, event: github.ReviewEventApprove, status: github.ReviewStatus{PullRequestIdentifier: pullRequestNodeIdentifier, HeadSHA: headSHA}})
	if inliner.view.review.submitting || len(inliner.client.Submissions()) != before {
		test.Errorf("a review started at its yes: submitting %v", inliner.view.review.submitting)
	}
	inliner.send(mergeSendMessage{flow: &inliner.view.merge, repository: testRepository, number: 7, method: github.MergeMethodMerge, status: github.MergeStatus{PullRequestIdentifier: pullRequestNodeIdentifier, HeadSHA: headSHA}})
	if inliner.view.merge.merging || len(inliner.client.Merges()) != 0 {
		test.Errorf("a merge started at its yes: merging %v", inliner.view.merge.merging)
	}
	if diff := cmp.Diff([]string{commentSavingText, commentSavingText}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
}

func TestInlineDraftKeyIncludesTheStartLine(test *testing.T) {
	inliner := newInliner(test)
	inliner.client.SetError(fake.MethodAddReviewThread, errors.New("boom"))
	inliner.body = "for 14 to 16"
	inliner.moveTo(github.SideRight, 14)
	inliner.press("V")
	inliner.typeText("jj")
	inliner.typeText("c")
	inliner.openedOn = nil
	inliner.moveTo(github.SideRight, 15) // 15 to 16 ends on the same line
	inliner.press("V")
	inliner.typeText("j")
	inliner.typeText("c")
	if diff := cmp.Diff([]string{""}, inliner.openedOn); diff != "" {
		test.Errorf("the draft of 14-16 was offered for 15-16 (-want +got):\n%s", diff)
	}
}

func TestAYesIsRefusedWhileAnotherWriteRuns(test *testing.T) {
	inliner := newInliner(test)
	inliner.view.writes = 1
	ran := false
	_, command := inliner.view.Update(commentAgreedMessage{origin: inliner.view.origin(), run: func() tea.Msg { ran = true; return nil }})
	inliner.run(command)
	if ran || inliner.view.writes != 1 {
		test.Errorf("the write ran %v, writes = %d", ran, inliner.view.writes)
	}
	if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
}

func TestTopLevelCommentIsRefusedWhileAWriteRuns(test *testing.T) {
	inliner := newInliner(test)
	inliner.press("h", "g g") // the Overview
	inliner.view.writes = 1
	inliner.messages, inliner.openedOn, inliner.body = nil, nil, "x"
	inliner.press("c")
	if len(inliner.openedOn) != 0 {
		test.Error("an editor opened while a write is running")
	}
	if diff := cmp.Diff([]string{commentSavingText}, inliner.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
}

// The editor may close after a review or write started: the text stays a draft.
func TestEditorClosingWhileBlockedKeepsTheText(test *testing.T) {
	for _, kind := range []targetKind{kindInline, kindEdit} {
		inliner := newInliner(test)
		inliner.view.review.submitting = true
		currentTarget := target{kind: kind, commentIdentifier: "c1", threadIdentifier: "t1", inline: &github.AddThreadInput{Path: clientPath, Side: github.SideRight, Line: 10}}
		inliner.send(editedMessage{owner: inliner.view, repository: testRepository, number: 7, sequence: inliner.view.commentSequence, target: currentTarget, body: "my text"})
		if got := inliner.methods(fake.MethodStartReview, fake.MethodAddReviewThread, fake.MethodUpdateReviewComment, fake.MethodGetReviewComment); len(got) != 0 {
			test.Errorf("kind %d: the client was called: %v", kind, got)
		}
		if got := inliner.view.drafts[currentTarget.draftKey()]; got != "my text" {
			test.Errorf("kind %d: draft = %q", kind, got)
		}
	}
}

func TestResultsNeverTakeTheCountBelowZero(test *testing.T) {
	inliner := newInliner(test)
	inliner.send(commentDeletedMessage{origin: inliner.view.origin()})
	inliner.send(postedMessage{origin: inliner.view.origin()})
	if inliner.view.writes != 0 {
		test.Errorf("writes = %d", inliner.view.writes)
	}
	inliner.view.writes = 1
	inliner.moveTo(github.SideRight, 10)
	inliner.press("c") // refused: one write is running
	if inliner.view.writes != 1 {
		test.Errorf("writes = %d, want 1", inliner.view.writes)
	}
}

// A diff reload in flight does not stop an inline comment that was already
// written: the head check covers it, and nothing is lost.
func TestInlineSendIgnoresADiffLoadInFlight(test *testing.T) {
	inliner := newInliner(test)
	inliner.body = "x"
	inliner.moveTo(github.SideRight, 10)
	inliner.view.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			inliner.view.diffPending = true // a reload of the same head is under way
			return done("x", nil)
		}
	}
	inliner.press("c")
	if len(inliner.client.AddedThreads()) != 1 {
		test.Errorf("added %d threads, error %q", len(inliner.client.AddedThreads()), inliner.lastError())
	}
}

func TestInlineFailedPostDeletesOnlyTheReviewItCreated(test *testing.T) {
	for _, testCase := range []struct {
		name          string
		existing      bool
		addError      error
		deleteError   error
		wantShown     string
		wantDeleted   int
		wantRemaining bool // a pending review is left on the server
	}{
		{name: "rejected comment deletes the created review", addError: fmt.Errorf("add: %w", github.ErrCommentRejected), wantShown: "comment not created", wantDeleted: 1},
		{name: "timeout leaves the review", addError: fmt.Errorf("add: %w", context.DeadlineExceeded), wantShown: "may have gone through", wantRemaining: true},
		{name: "generic error leaves the review", addError: errors.New("boom"), wantShown: "boom", wantRemaining: true},
		{name: "existing review is never deleted", existing: true, addError: github.ErrCommentRejected, wantShown: "comment not created", wantRemaining: true},
		{name: "delete failure keeps the original error", addError: github.ErrCommentRejected, deleteError: errors.New("delete exploded"), wantShown: "comment not created", wantRemaining: true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			if testCase.existing {
				inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "existing-review", CommitObjectIdentifier: inliner.view.diffHead})
			}
			inliner.client.SetError(fake.MethodAddReviewThread, testCase.addError)
			if testCase.deleteError != nil {
				inliner.client.SetError(fake.MethodDeleteReview, testCase.deleteError)
			}
			inliner.body = "my draft"
			inliner.moveTo(github.SideRight, 10)
			inliner.press("c")
			if got := inliner.lastError(); !strings.Contains(got, testCase.wantShown) || strings.Contains(got, "delete exploded") {
				test.Errorf("error = %q, want the add failure %q only", got, testCase.wantShown)
			}
			if got := len(inliner.client.DeletedReviews()); got != testCase.wantDeleted {
				test.Errorf("deleted %d reviews, want %d", got, testCase.wantDeleted)
			}
			status, err := inliner.client.GetReviewStatus(test.Context(), testRepository, 7)
			if err != nil || (status.Pending != nil) != testCase.wantRemaining {
				test.Errorf("pending review %+v, %v; want remaining %v", status.Pending, err, testCase.wantRemaining)
			}
			if testCase.existing && len(inliner.methods(fake.MethodDeleteReview)) != 0 {
				test.Errorf("DeleteReview called on an existing review")
			}
			if len(inliner.view.drafts) != 1 {
				test.Errorf("draft not kept: %v", inliner.view.drafts)
			}
		})
	}
}
