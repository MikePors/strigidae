package workspace

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// feed delivers msgs to v, and then the messages its commands produce.
func feed(view *View, messageList []tea.Msg) {
	for len(messageList) > 0 {
		message := messageList[0]
		messageList = messageList[1:]
		_, command := view.Update(message)
		messageList = append(messageList, messages(command)...)
	}
}

// A refresh after a push whose diff fetch fails leaves the old diff on
// screen, so the review must not go out for the new head.
func TestReviewDiffRefreshFailedMeansHeadMoved(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "ok"
	composer.client.SetHeadSHA(testRepository, 7, "bbbb2222")
	composer.client.SetError(fake.MethodGetDiff, errors.New("transient"))
	composer.press("R")
	if composer.v.detail.HeadSHA != "bbbb2222" || composer.v.diffHead != headSHA {
		test.Fatalf("detail head %q, diff head %q", composer.v.detail.HeadSHA, composer.v.diffHead)
	}
	composer.command("approve")
	configuration, ok := composer.lastConfirm()
	if !ok || !configuration.Notice {
		test.Fatalf("confirm = %+v, %v; want the head-moved notice", configuration, ok)
	}
	if len(composer.client.Submissions()) != 0 {
		test.Error("submitted")
	}
}

func TestReviewRefusedWhileDiffLoads(test *testing.T) {
	composer := newReviewer(test)
	composer.v.diffPending = true
	for _, function := range []func() tea.Cmd{
		func() tea.Cmd {
			return composer.v.review.start(composer.v.reviewEnvironment(), github.ReviewEventApprove)
		},
		func() tea.Cmd { return composer.v.review.openChooser(composer.v.reviewEnvironment()) },
	} {
		messageList := messages(function())
		if len(messageList) != 1 || messageList[0] != tea.Msg(userinterface.InfoMessage{Text: "diff not loaded"}) {
			test.Errorf("msgs = %#v", messageList)
		}
	}
	if composer.v.review.chooser != nil || len(composer.openedOn) != 0 {
		test.Error("review started")
	}
}

func TestReviewDiffHeadIsTheCommit(test *testing.T) {
	composer := newReviewer(test)
	composer.command("approve")
	configuration, _ := composer.lastConfirm()
	composer.run(configuration.OnYes)
	if subs := composer.client.Submissions(); len(subs) != 1 || subs[0].CommitObjectIdentifier != composer.v.diffHead || composer.v.diffHead != headSHA {
		test.Errorf("submissions = %+v, diff head %q", subs, composer.v.diffHead)
	}
}

func TestReviewSecondSubmitWhileInFlight(test *testing.T) {
	composer := newReviewer(test)
	composer.command("approve")
	configuration, _ := composer.lastConfirm()
	// The user said yes: handle the send message but do not deliver the result.
	messageList := messages(configuration.OnYes)
	_, send := composer.v.Update(messageList[0])
	if !composer.v.Loading() {
		test.Error("not loading while submitting")
	}
	calls := len(composer.client.Calls())
	got := messages(composer.v.review.start(composer.v.reviewEnvironment(), github.ReviewEventApprove))
	if len(got) != 1 || got[0] != tea.Msg(userinterface.InfoMessage{Text: "a review is being submitted"}) {
		test.Errorf("second start: %#v", got)
	}
	got = messages(composer.v.review.openChooser(composer.v.reviewEnvironment()))
	if len(got) != 1 || got[0] != tea.Msg(userinterface.InfoMessage{Text: "a review is being submitted"}) {
		test.Errorf("chooser: %#v", got)
	}
	if len(composer.client.Calls()) != calls || len(composer.openedOn) != 1 {
		test.Error("second review started")
	}
	composer.run(send) // the first one finishes
	if composer.v.review.submitting {
		test.Error("still submitting after the result")
	}
	if diff := cmp.Diff([]string{"Submitting review…", "Approved #7"}, composer.infoMessages()); diff != "" {
		test.Errorf("infos (-want +got):\n%s", diff)
	}
}

func TestReviewSubmitFailureRefreshes(test *testing.T) {
	composer := newReviewer(test)
	composer.body = "x"
	composer.command("approve")
	configuration, _ := composer.lastConfirm()
	composer.client.SetError(fake.MethodSubmitReview, errors.New("502 Bad Gateway"))
	before := len(composer.client.Calls())
	composer.run(configuration.OnYes)
	var reloaded bool
	for _, call := range composer.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded || composer.v.review.submitting {
		test.Errorf("reloaded=%v submitting=%v", reloaded, composer.v.review.submitting)
	}
}

// An empty body for a later verdict neither wipes the earlier draft nor is
// stored, and an empty approval sends an empty body.
func TestReviewEmptyBodyKeepsEarlierDraft(test *testing.T) {
	composer := newReviewer(test)
	composer.v.review.draft = "earlier thoughts"
	composer.body = ""
	composer.command("approve")
	if composer.v.review.draft != "earlier thoughts" {
		test.Errorf("draft = %q", composer.v.review.draft)
	}
	configuration, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	composer.run(configuration.OnYes)
	subs := composer.client.Submissions()
	if len(subs) != 1 || subs[0].Body != "" {
		test.Errorf("submissions = %+v, want an empty body", subs)
	}
	if !strings.Contains(strings.Join(composer.infoMessages(), ","), "Approved #7") {
		test.Errorf("infos = %v", composer.infoMessages())
	}
}
