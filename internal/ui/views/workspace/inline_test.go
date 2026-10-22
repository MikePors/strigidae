package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/components/diffview"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// Lines of internal/sync/client.go in testdata/pr.diff used below: new 10 is
// a context line, old 14 is the deleted `func Sync`, old 15 the deleted
// `return`, and new 14..16 are added lines.
const clientPath = "internal/sync/client.go"

// inliner is a reviewer with the diff file selected and the right pane focused.
type inliner struct{ *composer }

func newInliner(t *testing.T) *inliner {
	c := newReviewer(t)
	c.selectFile()
	return &inliner{c}
}

// moveTo puts the diff cursor on the line with the given number on side.
func (c *inliner) moveTo(side diffview.Side, n int) {
	c.t.Helper()
	dv := c.v.filePane.(*diffPane).dv
	dv.GotoTop()
	for range 200 {
		if l, ok := dv.Cursor(); ok && l.Side == side && l.Number() == n {
			return
		}
		dv.CursorDown(1)
	}
	c.t.Fatalf("no line %d on side %v", n, side)
}

// methods returns the recorded calls of the given fake methods.
func (c *inliner) methods(names ...string) []string {
	var out []string
	for _, call := range c.client.Calls() {
		for _, n := range names {
			if call.Method == n {
				out = append(out, n)
			}
		}
	}
	return out
}

func (c *inliner) pendingThread() github.ReviewThread {
	c.t.Helper()
	for _, th := range c.v.threads {
		for _, cm := range th.Comments {
			if cm.Pending {
				return th
			}
		}
	}
	c.t.Fatal("no pending thread shown")
	return github.ReviewThread{}
}

func TestInlineCommentOnEachSide(t *testing.T) {
	tests := []struct {
		name string
		side diffview.Side
		line int
		want github.AddThreadInput
	}{
		{"context line is on the right", diffview.SideRight, 10, github.AddThreadInput{Path: clientPath, Line: 10, Side: github.SideRight}},
		{"added line is on the right", diffview.SideRight, 14, github.AddThreadInput{Path: clientPath, Line: 14, Side: github.SideRight}},
		{"deleted line is on the left", diffview.SideLeft, 14, github.AddThreadInput{Path: clientPath, Line: 14, Side: github.SideLeft}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newInliner(t)
			c.body = "please rename\n"
			c.moveTo(tt.side, tt.line)
			c.press("c")
			if diff := cmp.Diff([]string{""}, c.openedOn); diff != "" {
				t.Fatalf("editor opened on (-want +got):\n%s", diff)
			}
			if _, asked := c.lastConfirm(); asked {
				t.Error("a pending comment must not ask for confirmation")
			}
			started := c.client.StartedReviews()
			if len(started) != 1 || started[0].CommitOID != headSHA {
				t.Fatalf("started = %+v", started)
			}
			got := c.client.AddedThreads()
			tt.want.Repo, tt.want.Number, tt.want.Body = testRepo, 7, "please rename"
			tt.want.PendingReviewID = started[0].ReviewID
			if diff := cmp.Diff([]github.AddThreadInput{tt.want}, got); diff != "" {
				t.Errorf("added (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"Pending comment added"}, c.infos()); diff != "" {
				t.Errorf("info (-want +got):\n%s", diff)
			}
			if len(c.v.drafts) != 0 {
				t.Errorf("draft kept: %v", c.v.drafts)
			}
			if th := c.pendingThread(); th.Line != tt.line || th.DiffSide != tt.want.Side {
				t.Errorf("shown thread = %+v", th)
			}
		})
	}
}

func TestInlineCommentsJoinTheSamePendingReview(t *testing.T) {
	c := newInliner(t)
	c.body = "one"
	c.moveTo(diffview.SideRight, 10)
	c.press("c")
	c.body = "two"
	c.moveTo(diffview.SideRight, 11)
	c.press("c")
	started, added := c.client.StartedReviews(), c.client.AddedThreads()
	if len(started) != 1 || len(added) != 2 {
		t.Fatalf("started %d reviews, added %d threads", len(started), len(added))
	}
	if added[0].PendingReviewID != started[0].ReviewID || added[1].PendingReviewID != started[0].ReviewID {
		t.Errorf("threads went to %q and %q, want %q", added[0].PendingReviewID, added[1].PendingReviewID, started[0].ReviewID)
	}
	if st, _ := c.client.GetReviewStatus(t.Context(), testRepo, 7); st.Pending == nil || st.Pending.CommentCount != 2 {
		t.Errorf("pending = %+v", st.Pending)
	}
}

func TestInlineCommentJoinsAnExistingPendingReview(t *testing.T) {
	c := newInliner(t)
	c.client.SetPendingReview(testRepo, 7, &github.PendingReview{ID: "PRR_x", CommitOID: headSHA, CommentCount: 3})
	c.body = "more"
	c.moveTo(diffview.SideRight, 10)
	c.press("c")
	if len(c.client.StartedReviews()) != 0 {
		t.Error("started a second review")
	}
	if got := c.client.AddedThreads(); len(got) != 1 || got[0].PendingReviewID != "PRR_x" {
		t.Errorf("added = %+v", got)
	}
}

func TestInlineMultiLineComment(t *testing.T) {
	c := newInliner(t)
	c.body = "this whole block"
	c.moveTo(diffview.SideRight, 14)
	c.press("V")
	if !c.v.CapturesKeys() {
		t.Fatal("visual mode does not capture keys")
	}
	c.typ("jj")
	if out := ansi.Strip(c.render()); strings.Count(out, markSelectedForTest) != 2 {
		t.Errorf("want 2 selected rows marked plus the cursor row:\n%s", out)
	}
	c.typ("c")
	if c.v.CapturesKeys() {
		t.Error("visual mode still active after c")
	}
	want := github.AddThreadInput{
		Repo: testRepo, Number: 7, Path: clientPath, Line: 16, Side: github.SideRight,
		StartLine: 14, StartSide: github.SideRight, Body: "this whole block",
		PendingReviewID: c.client.StartedReviews()[0].ReviewID,
	}
	if diff := cmp.Diff([]github.AddThreadInput{want}, c.client.AddedThreads()); diff != "" {
		t.Errorf("added (-want +got):\n%s", diff)
	}
	if th := c.pendingThread(); th.StartLine != 14 || th.Line != 16 {
		t.Errorf("shown thread = %+v", th)
	}
}

// markSelectedForTest is diffview's gutter mark of selected rows.
const markSelectedForTest = "▎"

func TestVisualRefusesTwoSidesAndEscLeaves(t *testing.T) {
	c := newInliner(t)
	c.moveTo(diffview.SideLeft, 15)
	c.press("V")
	c.msgs = nil
	c.typ("j") // the next line is an added one
	if got := c.infos(); len(got) != 1 || !strings.Contains(got[0], "one side") {
		t.Errorf("info = %v", got)
	}
	c.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if c.v.CapturesKeys() || c.v.filePane.VisualActive() {
		t.Error("esc did not leave visual mode")
	}
	if len(c.openedOn) != 0 {
		t.Error("esc opened the editor")
	}
}

func TestVisualOnHunkHeaderDoesNotStart(t *testing.T) {
	c := newInliner(t)
	c.v.filePane.(*diffPane).dv.GotoTop()
	c.msgs = nil
	c.press("V")
	if c.v.CapturesKeys() {
		t.Error("visual mode started on a hunk header")
	}
	if got := c.infos(); len(got) != 1 || !strings.Contains(got[0], "diff line") {
		t.Errorf("info = %v", got)
	}
}

// TestVisualReleasesOnEscThroughRoot is the workspace's entry for the
// esc-releases-capture rule for visual mode, driven through the real root.
func TestVisualReleasesOnEscThroughRoot(t *testing.T) {
	c := newInliner(t)
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	m := ui.New(c.v, theme.New(true), ui.Options{Tick: noTick})
	step := func(msg tea.Msg) { _, _ = m.Update(msg) }
	step(tea.WindowSizeMsg{Width: 110, Height: 41})
	c.moveTo(diffview.SideRight, 10)
	step(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if !c.v.CapturesKeys() {
		t.Fatal("V through the root did not start visual mode")
	}
	step(tea.KeyPressMsg{Code: 'x', Text: "x"}) // keys are raw while captured: x is not a command
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if c.v.CapturesKeys() {
		t.Error("view still captures keys after esc")
	}
}

func TestInlineEmptyBodyIsAnError(t *testing.T) {
	for _, body := range []string{"", " \n\t"} {
		c := newInliner(t)
		c.body = body
		c.moveTo(diffview.SideRight, 10)
		c.press("c")
		if !strings.Contains(c.lastError(), "empty comment") {
			t.Errorf("body %q: error = %q", body, c.lastError())
		}
		if got := c.methods(fake.MethodStartReview, fake.MethodAddReviewThread); len(got) != 0 {
			t.Errorf("body %q: client called: %v", body, got)
		}
	}
}

func TestInlineFailedPostKeepsDraft(t *testing.T) {
	c := newInliner(t)
	c.client.SetError(fake.MethodAddReviewThread, errors.New("boom"))
	c.body = "my draft"
	c.moveTo(diffview.SideRight, 10)
	c.press("c")
	if !strings.Contains(c.lastError(), "boom") || !strings.Contains(c.lastError(), "draft") {
		t.Errorf("error = %q", c.lastError())
	}
	if len(c.v.drafts) != 1 {
		t.Fatalf("drafts = %v", c.v.drafts)
	}
	c.client.SetError(fake.MethodAddReviewThread, nil)
	c.press("c")
	if diff := cmp.Diff([]string{"", "my draft"}, c.openedOn); diff != "" {
		t.Errorf("second editor (-want +got):\n%s", diff)
	}
	if len(c.client.AddedThreads()) != 1 || len(c.v.drafts) != 0 {
		t.Errorf("retry: added %d, drafts %v", len(c.client.AddedThreads()), c.v.drafts)
	}
	// The failed first try had already created the pending review; the retry joins it.
	if len(c.client.StartedReviews()) != 1 {
		t.Errorf("started %d reviews", len(c.client.StartedReviews()))
	}
}

func TestInlineEditorFailureKeepsTypedText(t *testing.T) {
	c := newInliner(t)
	c.body, c.err = "typed before the crash", errors.New("exit status 1")
	c.moveTo(diffview.SideRight, 10)
	c.press("c")
	if len(c.v.drafts) != 1 || len(c.client.AddedThreads()) != 0 {
		t.Errorf("drafts %v, added %d", c.v.drafts, len(c.client.AddedThreads()))
	}
}

func TestInlineRefusals(t *testing.T) {
	t.Run("diff not loaded", func(t *testing.T) {
		c := newReviewer(t)
		c.selectFile()
		c.v.diff = DiffState{Err: errors.New("nope")} // as after a first load that failed
		c.msgs, c.body = nil, "x"
		c.press("c")
		if got := c.infos(); len(got) != 1 || got[0] != "diff not loaded" || len(c.openedOn) != 0 {
			t.Errorf("infos %v, opened %v", got, c.openedOn)
		}
	})
	t.Run("head moved", func(t *testing.T) {
		c := newInliner(t)
		c.client.SetHeadSHA(testRepo, 7, "bbbb2222")
		c.body = "x"
		c.moveTo(diffview.SideRight, 10)
		c.press("c")
		if !strings.Contains(c.lastError(), "head moved") {
			t.Errorf("error = %q", c.lastError())
		}
		if got := c.methods(fake.MethodStartReview, fake.MethodAddReviewThread); len(got) != 0 || len(c.v.drafts) != 1 {
			t.Errorf("calls %v, drafts %v", got, c.v.drafts)
		}
	})
	t.Run("pending review on another commit", func(t *testing.T) {
		c := newInliner(t)
		c.client.SetPendingReview(testRepo, 7, &github.PendingReview{ID: "PRR_old", CommitOID: "old1111"})
		c.body = "x"
		c.moveTo(diffview.SideRight, 10)
		c.press("c")
		if !strings.Contains(c.lastError(), "older commit") || len(c.client.AddedThreads()) != 0 {
			t.Errorf("error %q, added %d", c.lastError(), len(c.client.AddedThreads()))
		}
	})
	t.Run("hunk header", func(t *testing.T) {
		c := newInliner(t)
		c.v.filePane.(*diffPane).dv.GotoTop()
		c.msgs = nil
		c.press("c")
		if got := c.infos(); len(got) != 1 || len(c.openedOn) != 0 {
			t.Errorf("infos %v, opened %v", got, c.openedOn)
		}
	})
}

// addPending adds a comment on new line 10 and moves the cursor onto its thread.
func (c *inliner) addPending(body string) github.ReviewThread {
	c.t.Helper()
	c.body = body
	c.moveTo(diffview.SideRight, 10)
	c.press("c")
	th := c.pendingThread()
	dv := c.v.filePane.(*diffPane).dv
	for range 200 {
		if ts := dv.ThreadsAtCursor(); len(ts) == 1 && ts[0].ID == th.ID {
			return th
		}
		dv.CursorDown(1)
	}
	c.t.Fatal("cannot reach the pending thread")
	return th
}

func TestEditPendingComment(t *testing.T) {
	c := newInliner(t)
	th := c.addPending("first draft")
	c.openedOn, c.msgs = nil, nil
	c.body = "second draft"
	c.press("e")
	if diff := cmp.Diff([]string{"first draft"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	if _, asked := c.lastConfirm(); asked {
		t.Error("editing a pending comment must not ask for confirmation")
	}
	if got := c.pendingThread().Comments[0].Body; got != "second draft" {
		t.Errorf("body = %q", got)
	}
	if diff := cmp.Diff([]string{"Comment updated"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
	if th.ID != c.pendingThread().ID {
		t.Error("edit changed the thread")
	}
}

func TestEditPendingEmptyBodyAndUnchanged(t *testing.T) {
	c := newInliner(t)
	c.addPending("keep me")
	c.body = ""
	c.press("e")
	if !strings.Contains(c.lastError(), "empty comment") {
		t.Errorf("error = %q", c.lastError())
	}
	c.client.SetError(fake.MethodUpdateReviewComment, errors.New("boom"))
	c.body = "changed"
	c.press("e")
	if len(c.v.drafts) != 1 || !strings.Contains(c.lastError(), "boom") {
		t.Errorf("failed edit: drafts %v, error %q", c.v.drafts, c.lastError())
	}
	c.client.SetError(fake.MethodUpdateReviewComment, nil)
	c.msgs, c.body = nil, "keep me"
	c.press("e") // the kept draft replaces the body in the editor; the editor returns the original text
	if got := c.infos(); len(got) != 1 || got[0] != "comment unchanged" {
		t.Errorf("info = %v", got)
	}
}

func TestEditAndDeleteOnlyYourComments(t *testing.T) {
	c := newInliner(t)
	c.press(")") // a thread by someone else
	c.msgs = nil
	c.press("e")
	c.press("D")
	if got := c.infos(); len(got) != 2 || got[0] != "no comment of yours here" || got[1] != got[0] {
		t.Errorf("infos = %v", got)
	}
	if len(c.openedOn) != 0 {
		t.Error("editor opened on someone else's comment")
	}
	c.v.filePane.(*diffPane).dv.GotoTop() // a hunk header: no thread at all
	c.msgs = nil
	c.press("e")
	if got := c.infos(); len(got) != 1 || got[0] != "no comment of yours here" {
		t.Errorf("infos on a line without a thread = %v", got)
	}
}

func TestDeletePendingComment(t *testing.T) {
	c := newInliner(t)
	th := c.addPending("oops")
	c.msgs = nil
	c.press("D")
	conf, ok := c.lastConfirm()
	if !ok || conf.Text != "Delete pending comment on "+clientPath+":10?" {
		t.Fatalf("confirmation = %q (%v)", conf.Text, ok)
	}
	if len(c.client.DeletedComments()) != 0 {
		t.Fatal("deleted before the answer")
	}
	c.run(conf.OnYes)
	if diff := cmp.Diff([]string{th.Comments[0].ID}, c.client.DeletedComments()); diff != "" {
		t.Errorf("deleted (-want +got):\n%s", diff)
	}
	for _, t2 := range c.v.threads {
		if t2.ID == th.ID {
			t.Error("deleted thread still shown")
		}
	}
	if diff := cmp.Diff([]string{"Comment deleted"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
}

func TestDeletePendingFailureIsReported(t *testing.T) {
	c := newInliner(t)
	c.addPending("oops")
	c.client.SetError(fake.MethodDeleteReviewComment, errors.New("boom"))
	c.msgs = nil
	c.press("D")
	conf, _ := c.lastConfirm()
	c.run(conf.OnYes)
	if !strings.Contains(c.lastError(), "boom") {
		t.Errorf("error = %q", c.lastError())
	}
}

func TestInlineKeysAreInHelp(t *testing.T) {
	h := loaded(t, 100, 30)
	got := map[string]bool{}
	for _, g := range h.v.FullHelp() {
		for _, b := range g {
			if k := b.Keys(); len(k) == 1 {
				got[k[0]] = true
			}
		}
	}
	for _, k := range []string{"V", "e", "D"} {
		if !got[k] {
			t.Errorf("%s is not in the help", k)
		}
	}
}

// submitElsewhere publishes the pending review on the server, as submitting
// it on github.com would, without the view hearing about it.
func (c *inliner) submitElsewhere() {
	c.t.Helper()
	in := github.SubmitReviewInput{
		Repo: testRepo, Number: 7, PullRequestID: prNodeID, Event: github.ReviewEventComment,
		PendingReviewID: c.client.StartedReviews()[0].ReviewID,
	}
	if err := c.client.SubmitReview(c.t.Context(), in); err != nil {
		c.t.Fatal(err)
	}
}

func (c *inliner) serverBody(id string) string {
	c.t.Helper()
	page, err := c.client.ListReviewThreads(c.t.Context(), testRepo, 7, github.PageRequest{})
	if err != nil {
		c.t.Fatal(err)
	}
	for _, th := range page.Items {
		for _, cm := range th.Comments {
			if cm.ID == id {
				return cm.Body
			}
		}
	}
	c.t.Fatalf("comment %s not on the server", id)
	return ""
}

// mine returns a published thread by the viewer on new line 10 of client.go.
func (c *inliner) mine(canUpdate, canDelete bool, comments ...github.ReviewComment) github.ReviewThread {
	if len(comments) == 0 {
		comments = []github.ReviewComment{{ID: "c-mine", Author: c.client.Viewer, Body: "published note"}}
	}
	for i := range comments {
		comments[i].Author = c.client.Viewer
		comments[i].ViewerCanUpdate, comments[i].ViewerCanDelete = canUpdate, canDelete
	}
	return github.ReviewThread{ID: "t-mine", Path: clientPath, Line: 10, DiffSide: github.SideRight, Comments: comments}
}

// seed sets the server's threads (the shared ones plus extra) and refreshes the view.
func (c *inliner) seed(extra ...github.ReviewThread) {
	c.t.Helper()
	c.client.SetReviewThreads(testRepo, 7, append(testThreads(), extra...))
	c.press("R")
}

// onMine moves the diff cursor onto the thread made by mine.
func (c *inliner) onMine() {
	const id = "t-mine"
	c.t.Helper()
	dv := c.v.filePane.(*diffPane).dv
	dv.GotoTop()
	for range 200 {
		if ts := dv.ThreadsAtCursor(); len(ts) == 1 && ts[0].ID == id {
			return
		}
		dv.CursorDown(1)
	}
	c.t.Fatalf("cannot reach thread %s", id)
}

func (c *inliner) updates() []string { return c.methods(fake.MethodUpdateReviewComment) }
func (c *inliner) deletes() []string { return c.methods(fake.MethodDeleteReviewComment) }

func TestEditPublishedCommentAsksAfterTheEditor(t *testing.T) {
	c := newInliner(t)
	c.seed(c.mine(true, true))
	c.onMine()
	c.msgs, c.body = nil, "published, revised"
	c.press("e")
	if diff := cmp.Diff([]string{"published note"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
	conf, ok := c.lastConfirm()
	if !ok || conf.Text != "Update your published comment on "+clientPath+":10?" {
		t.Fatalf("confirmation = %q (%v)", conf.Text, ok)
	}
	if len(c.updates()) != 0 {
		t.Fatal("updated before the answer")
	}
	c.run(conf.OnYes)
	if got := c.serverBody("c-mine"); got != "published, revised" {
		t.Errorf("body = %q", got)
	}
	if diff := cmp.Diff([]string{"Comment updated"}, c.infos()); diff != "" {
		t.Errorf("info (-want +got):\n%s", diff)
	}
}

func TestDeleteAlwaysAsks(t *testing.T) {
	t.Run("published", func(t *testing.T) {
		c := newInliner(t)
		c.seed(c.mine(true, true))
		c.onMine()
		c.msgs = nil
		c.press("D")
		conf, ok := c.lastConfirm()
		if !ok || conf.Text != "Delete your published comment on "+clientPath+":10?" {
			t.Fatalf("confirmation = %q (%v)", conf.Text, ok)
		}
		if len(c.deletes()) != 0 {
			t.Fatal("deleted before the answer")
		}
		c.run(conf.OnYes)
		if diff := cmp.Diff([]string{"c-mine"}, c.client.DeletedComments()); diff != "" {
			t.Errorf("deleted (-want +got):\n%s", diff)
		}
	})
	t.Run("pending", func(t *testing.T) { // TestDeletePendingComment covers the details
		c := newInliner(t)
		c.addPending("x")
		c.msgs = nil
		c.press("D")
		if conf, ok := c.lastConfirm(); !ok || !strings.HasPrefix(conf.Text, "Delete pending comment on ") {
			t.Errorf("confirmation = %q (%v)", conf.Text, ok)
		}
	})
}

func TestPendingCommentIsPreferredOverAPublishedOne(t *testing.T) {
	c := newInliner(t)
	c.seed(c.mine(true, true,
		github.ReviewComment{ID: "c-pub", Body: "published"},
		github.ReviewComment{ID: "c-pend", Body: "draft", Pending: true},
		github.ReviewComment{ID: "c-pub2", Body: "published later"},
	))
	c.client.SetPendingReview(testRepo, 7, &github.PendingReview{ID: "PRR_x", CommitOID: headSHA})
	c.onMine()
	c.body = "draft"
	c.press("e")
	if diff := cmp.Diff([]string{"draft"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

func TestLastChangeableCommentWithoutAPendingOne(t *testing.T) {
	c := newInliner(t)
	other := github.ReviewComment{ID: "c-other", Author: "bob", Body: "theirs"}
	th := c.mine(true, true, github.ReviewComment{ID: "c-first", Body: "first"}, github.ReviewComment{ID: "c-second", Body: "second"})
	th.Comments = append(th.Comments, other) // the last comment is not the viewer's
	c.seed(th)
	c.onMine()
	c.body = "second"
	c.press("e")
	if diff := cmp.Diff([]string{"second"}, c.openedOn); diff != "" {
		t.Errorf("editor opened on (-want +got):\n%s", diff)
	}
}

// TestPendingTurnedPublishedWhileTheEditorIsOpen: fresh state decides, so a
// comment that was pending when e was pressed is never silently updated once
// it has been published.
func TestPendingTurnedPublishedWhileTheEditorIsOpen(t *testing.T) {
	c := newInliner(t)
	id := c.addPending("original").Comments[0].ID
	c.v.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
		return func() tea.Msg {
			c.submitElsewhere() // submitted on github.com while the editor is open
			return done("rewritten", nil)
		}
	}
	c.msgs = nil
	c.press("e")
	if len(c.updates()) != 0 {
		t.Fatal("a published comment was updated without asking")
	}
	conf, ok := c.lastConfirm()
	if !ok || conf.Text != "Update your published comment on "+clientPath+":10?" {
		t.Fatalf("confirmation = %q (%v)", conf.Text, ok)
	}
	c.run(conf.OnYes)
	if got := c.serverBody(id); got != "rewritten" {
		t.Errorf("body = %q", got)
	}
}

func TestPendingTurnedPublishedBeforeDeleteAsksTheRightQuestion(t *testing.T) {
	c := newInliner(t)
	c.addPending("original")
	c.submitElsewhere() // the view still shows it as pending
	c.msgs = nil
	c.press("D")
	if conf, ok := c.lastConfirm(); !ok || !strings.HasPrefix(conf.Text, "Delete your published comment on ") {
		t.Errorf("confirmation = %q (%v)", conf.Text, ok)
	}
}

func TestGoneOrNoLongerYoursIsRefusedAndRefreshed(t *testing.T) {
	gone := map[string]func(c *inliner, id string){
		"deleted elsewhere": func(c *inliner, id string) {
			if err := c.client.DeleteReviewComment(t.Context(), id); err != nil {
				t.Fatal(err)
			}
		},
		"no longer yours": func(c *inliner, id string) {
			c.client.SetReviewThreads(testRepo, 7, append(testThreads(), c.mine(false, false, github.ReviewComment{ID: id, Body: "published note"})))
		},
	}
	for name, change := range gone {
		t.Run(name+"/edit", func(t *testing.T) {
			c := newInliner(t)
			c.seed(c.mine(true, true))
			c.onMine()
			c.msgs, c.body = nil, "rewritten"
			c.v.edit = func(_ string, done func(string, error) tea.Msg) tea.Cmd {
				return func() tea.Msg {
					change(c, "c-mine")
					return done("rewritten", nil)
				}
			}
			c.press("e")
			if !strings.Contains(c.lastError(), "gone or no longer yours") {
				t.Errorf("error = %q", c.lastError())
			}
			if _, asked := c.lastConfirm(); asked || len(c.updates()) != 0 {
				t.Errorf("asked %v, updates %v", asked, c.updates())
			}
		})
		t.Run(name+"/delete", func(t *testing.T) {
			c := newInliner(t)
			c.seed(c.mine(true, true))
			c.onMine()
			c.msgs = nil
			change(c, "c-mine")
			before := len(c.deletes())
			c.press("D")
			if !strings.Contains(c.lastError(), "gone or no longer yours") {
				t.Errorf("error = %q", c.lastError())
			}
			if _, asked := c.lastConfirm(); asked || len(c.deletes()) != before {
				t.Errorf("asked %v, deletes %v", asked, c.deletes())
			}
		})
	}
}

func TestDeleteRechecksAfterTheAnswer(t *testing.T) {
	c := newInliner(t)
	c.addPending("original")
	c.msgs = nil
	c.press("D")
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no confirmation")
	}
	// Someone deletes it while the question is open.
	if err := c.client.DeleteReviewComment(t.Context(), c.pendingThread().Comments[0].ID); err != nil {
		t.Fatal(err)
	}
	before := len(c.deletes())
	c.run(conf.OnYes)
	if len(c.deletes()) != before {
		t.Error("delete sent for a comment that is gone")
	}
	if !strings.Contains(c.lastError(), "gone or no longer yours") {
		t.Errorf("error = %q", c.lastError())
	}
}

func TestCommandsBehaveLikeTheKeys(t *testing.T) {
	t.Run("edit pending", func(t *testing.T) {
		c := newInliner(t)
		id := c.addPending("first draft").Comments[0].ID
		c.openedOn, c.msgs, c.body = nil, nil, "second draft"
		c.command("edit-comment")
		if diff := cmp.Diff([]string{"first draft"}, c.openedOn); diff != "" {
			t.Errorf("editor opened on (-want +got):\n%s", diff)
		}
		if _, asked := c.lastConfirm(); asked {
			t.Error("a pending edit asked for confirmation")
		}
		if got := c.serverBody(id); got != "second draft" {
			t.Errorf("body = %q", got)
		}
	})
	t.Run("edit published", func(t *testing.T) {
		c := newInliner(t)
		c.seed(c.mine(true, true))
		c.onMine()
		c.msgs, c.body = nil, "revised"
		c.command("edit-comment")
		if conf, ok := c.lastConfirm(); !ok || !strings.HasPrefix(conf.Text, "Update your published comment") {
			t.Errorf("confirmation = %q (%v)", conf.Text, ok)
		}
	})
	t.Run("delete", func(t *testing.T) {
		c := newInliner(t)
		c.addPending("x")
		c.msgs = nil
		c.command("delete-comment")
		if conf, ok := c.lastConfirm(); !ok || !strings.HasPrefix(conf.Text, "Delete pending comment on ") {
			t.Errorf("confirmation = %q (%v)", conf.Text, ok)
		}
	})
	t.Run("in the overview", func(t *testing.T) {
		c := newInliner(t)
		c.press("h", "g g") // Files pane, Overview
		c.msgs = nil
		c.command("edit-comment")
		if got := c.infos(); len(got) != 1 || got[0] != "no comment of yours here" {
			t.Errorf("infos = %v", got)
		}
	})
}

func TestEditAndDeleteRefusedWhileSubmitting(t *testing.T) {
	c := newInliner(t)
	c.addPending("original")
	c.openedOn, c.msgs = nil, nil
	c.v.review.submitting = true
	c.body = "x"
	c.press("e")
	c.press("D")
	if len(c.openedOn) != 0 {
		t.Error("editor opened while a review is being submitted")
	}
	if got := c.infos(); len(got) != 2 || got[0] != "a review is being submitted" {
		t.Errorf("infos = %v", got)
	}
	if _, asked := c.lastConfirm(); asked {
		t.Error("asked to delete while a review is being submitted")
	}
	if got := c.methods(fake.MethodUpdateReviewComment, fake.MethodDeleteReviewComment); len(got) != 0 {
		t.Errorf("mutations sent: %v", got)
	}
}
