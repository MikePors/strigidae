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
	if composer.view.detail.HeadSHA != "bbbb2222" || composer.view.diffHead != headSHA {
		test.Fatalf("detail head %q, diff head %q", composer.view.detail.HeadSHA, composer.view.diffHead)
	}
	composer.command("approve")
	confirmation, ok := composer.lastConfirm()
	if !ok || !confirmation.Notice {
		test.Fatalf("confirm = %+v, %v; want the head-moved notice", confirmation, ok)
	}
	if len(composer.client.Submissions()) != 0 {
		test.Error("submitted")
	}
}

func TestReviewRefusedWhileDiffLoads(test *testing.T) {
	composer := newReviewer(test)
	composer.view.diffPending = true
	for _, function := range []func() tea.Cmd{
		func() tea.Cmd {
			return composer.view.review.start(composer.view.reviewEnvironment(), github.ReviewEventApprove)
		},
		func() tea.Cmd { return composer.view.review.openChooser(composer.view.reviewEnvironment()) },
	} {
		messageList := messages(function())
		if len(messageList) != 1 || messageList[0] != tea.Msg(userinterface.InfoMessage{Text: "diff not loaded"}) {
			test.Errorf("msgs = %#v", messageList)
		}
	}
	if composer.view.review.chooser != nil || len(composer.openedOn) != 0 {
		test.Error("review started")
	}
}

func TestReviewDiffHeadIsTheCommit(test *testing.T) {
	composer := newReviewer(test)
	composer.command("approve")
	confirmation, _ := composer.lastConfirm()
	composer.run(confirmation.OnYes)
	if submissions := composer.client.Submissions(); len(submissions) != 1 || submissions[0].CommitObjectIdentifier != composer.view.diffHead || composer.view.diffHead != headSHA {
		test.Errorf("submissions = %+v, diff head %q", submissions, composer.view.diffHead)
	}
}

func TestReviewSecondSubmitWhileInFlight(test *testing.T) {
	composer := newReviewer(test)
	composer.command("approve")
	confirmation, _ := composer.lastConfirm()
	// The user said yes: handle the send message but do not deliver the result.
	messageList := messages(confirmation.OnYes)
	_, send := composer.view.Update(messageList[0])
	if !composer.view.Loading() {
		test.Error("not loading while submitting")
	}
	calls := len(composer.client.Calls())
	got := messages(composer.view.review.start(composer.view.reviewEnvironment(), github.ReviewEventApprove))
	if len(got) != 1 || got[0] != tea.Msg(userinterface.InfoMessage{Text: "a review is being submitted"}) {
		test.Errorf("second start: %#v", got)
	}
	got = messages(composer.view.review.openChooser(composer.view.reviewEnvironment()))
	if len(got) != 1 || got[0] != tea.Msg(userinterface.InfoMessage{Text: "a review is being submitted"}) {
		test.Errorf("chooser: %#v", got)
	}
	if len(composer.client.Calls()) != calls || len(composer.openedOn) != 1 {
		test.Error("second review started")
	}
	composer.run(send) // the first one finishes
	if composer.view.review.submitting {
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
	confirmation, _ := composer.lastConfirm()
	composer.client.SetError(fake.MethodSubmitReview, errors.New("502 Bad Gateway"))
	before := len(composer.client.Calls())
	composer.run(confirmation.OnYes)
	var reloaded bool
	for _, call := range composer.client.Calls()[before:] {
		reloaded = reloaded || call.Method == fake.MethodGetPullRequest
	}
	if !reloaded || composer.view.review.submitting {
		test.Errorf("reloaded=%v submitting=%v", reloaded, composer.view.review.submitting)
	}
}

// An empty body for a later verdict neither wipes the earlier draft nor is
// stored, and an empty approval sends an empty body.
func TestReviewEmptyBodyKeepsEarlierDraft(test *testing.T) {
	composer := newReviewer(test)
	composer.view.review.draft = "earlier thoughts"
	composer.body = ""
	composer.command("approve")
	if composer.view.review.draft != "earlier thoughts" {
		test.Errorf("draft = %q", composer.view.review.draft)
	}
	confirmation, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	composer.run(confirmation.OnYes)
	submissions := composer.client.Submissions()
	if len(submissions) != 1 || submissions[0].Body != "" {
		test.Errorf("submissions = %+v, want an empty body", submissions)
	}
	if !strings.Contains(strings.Join(composer.infoMessages(), ","), "Approved #7") {
		test.Errorf("infos = %v", composer.infoMessages())
	}
}
