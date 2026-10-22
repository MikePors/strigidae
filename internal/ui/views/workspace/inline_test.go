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
	if diff := cmp.Diff([]string{"Pending comment updated"}, c.infos()); diff != "" {
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

func TestEditAndDeleteOnlyYourPendingComments(t *testing.T) {
	c := newInliner(t)
	c.press(")") // a submitted thread
	c.msgs = nil
	c.press("e")
	c.press("D")
	if got := c.infos(); len(got) != 2 || !strings.Contains(got[0], "no pending comment") {
		t.Errorf("infos = %v", got)
	}
	if len(c.openedOn) != 0 {
		t.Error("editor opened on a submitted comment")
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
	if diff := cmp.Diff([]string{"Pending comment deleted"}, c.infos()); diff != "" {
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
