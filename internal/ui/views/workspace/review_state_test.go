package workspace

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
)

// feed delivers msgs to v, and then the messages its commands produce.
func feed(v *View, msgs []tea.Msg) {
	for len(msgs) > 0 {
		m := msgs[0]
		msgs = msgs[1:]
		_, cmd := v.Update(m)
		msgs = append(msgs, messages(cmd)...)
	}
}

// A refresh after a push whose diff fetch fails leaves the old diff on
// screen, so the review must not go out for the new head.
func TestReviewDiffRefreshFailedMeansHeadMoved(t *testing.T) {
	c := newReviewer(t)
	c.body = "ok"
	c.client.SetHeadSHA(testRepo, 7, "bbbb2222")
	c.client.SetError(fake.MethodGetDiff, errors.New("transient"))
	c.press("R")
	if c.v.detail.HeadSHA != "bbbb2222" || c.v.diffHead != headSHA {
		t.Fatalf("detail head %q, diff head %q", c.v.detail.HeadSHA, c.v.diffHead)
	}
	c.command("approve")
	conf, ok := c.lastConfirm()
	if !ok || !conf.Notice {
		t.Fatalf("confirm = %+v, %v; want the head-moved notice", conf, ok)
	}
	if len(c.client.Submissions()) != 0 {
		t.Error("submitted")
	}
}

func TestReviewRefusedWhileDiffLoads(t *testing.T) {
	c := newReviewer(t)
	c.v.diffPending = true
	for _, f := range []func() tea.Cmd{
		func() tea.Cmd { return c.v.startReview(github.ReviewEventApprove) },
		c.v.openChooser,
	} {
		msgs := messages(f())
		if len(msgs) != 1 || msgs[0] != tea.Msg(ui.InfoMsg{Text: "diff not loaded"}) {
			t.Errorf("msgs = %#v", msgs)
		}
	}
	if c.v.chooser != nil || len(c.openedOn) != 0 {
		t.Error("review started")
	}
}

func TestReviewDiffHeadIsTheCommit(t *testing.T) {
	c := newReviewer(t)
	c.command("approve")
	conf, _ := c.lastConfirm()
	c.run(conf.OnYes)
	if subs := c.client.Submissions(); len(subs) != 1 || subs[0].CommitOID != c.v.diffHead || c.v.diffHead != headSHA {
		t.Errorf("submissions = %+v, diff head %q", subs, c.v.diffHead)
	}
}

func TestReviewSecondSubmitWhileInFlight(t *testing.T) {
	c := newReviewer(t)
	c.command("approve")
	conf, _ := c.lastConfirm()
	// The user said yes: handle the send message but do not deliver the result.
	msgs := messages(conf.OnYes)
	_, send := c.v.Update(msgs[0])
	if !c.v.Loading() {
		t.Error("not loading while submitting")
	}
	calls := len(c.client.Calls())
	got := messages(c.v.startReview(github.ReviewEventApprove))
	if len(got) != 1 || got[0] != tea.Msg(ui.InfoMsg{Text: "a review is being submitted"}) {
		t.Errorf("second start: %#v", got)
	}
	got = messages(c.v.openChooser())
	if len(got) != 1 || got[0] != tea.Msg(ui.InfoMsg{Text: "a review is being submitted"}) {
		t.Errorf("chooser: %#v", got)
	}
	if len(c.client.Calls()) != calls || len(c.openedOn) != 1 {
		t.Error("second review started")
	}
	c.run(send) // the first one finishes
	if c.v.submitting {
		t.Error("still submitting after the result")
	}
	if diff := cmp.Diff([]string{"Submitting review…", "Approved #7"}, c.infos()); diff != "" {
		t.Errorf("infos (-want +got):\n%s", diff)
	}
}

func TestReviewSubmitFailureRefreshes(t *testing.T) {
	c := newReviewer(t)
	c.body = "x"
	c.command("approve")
	conf, _ := c.lastConfirm()
	c.client.SetError(fake.MethodSubmitReview, errors.New("502 Bad Gateway"))
	before := len(c.client.Calls())
	c.run(conf.OnYes)
	var reloaded bool
	for _, call := range c.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded || c.v.submitting {
		t.Errorf("reloaded=%v submitting=%v", reloaded, c.v.submitting)
	}
}

// An empty body for a later verdict neither wipes the earlier draft nor is
// stored, and an empty approval sends an empty body.
func TestReviewEmptyBodyKeepsEarlierDraft(t *testing.T) {
	c := newReviewer(t)
	c.v.reviewDraft = "earlier thoughts"
	c.body = ""
	c.command("approve")
	if c.v.reviewDraft != "earlier thoughts" {
		t.Errorf("draft = %q", c.v.reviewDraft)
	}
	conf, ok := c.lastConfirm()
	if !ok {
		t.Fatal("no confirmation")
	}
	c.run(conf.OnYes)
	subs := c.client.Submissions()
	if len(subs) != 1 || subs[0].Body != "" {
		t.Errorf("submissions = %+v, want an empty body", subs)
	}
	if !strings.Contains(strings.Join(c.infos(), ","), "Approved #7") {
		t.Errorf("infos = %v", c.infos())
	}
}
