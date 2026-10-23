package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

// newOwnReviewer is a reviewer whose pull request the viewer authored.
func newOwnReviewer(test *testing.T) *composer {
	composer := newComposer(test)
	detail := testDetail()
	detail.Identifier = pullRequestNodeIdentifier
	detail.ViewerDidAuthor = true
	composer.client.SetPullRequest(detail)
	composer.setHead(headSHA)
	return composer
}

func TestReviewChooserOnOwnPullRequest(test *testing.T) {
	own := newOwnReviewer(test)
	own.command("review")
	if own.view.review.chooser == nil {
		test.Fatal(":review did not open the chooser")
	}
	rendered := own.render()
	for _, absent := range []string{"Approve", "Request changes"} {
		if strings.Contains(rendered, absent) {
			test.Errorf("own pull request chooser offers %q:\n%s", absent, rendered)
		}
	}
	for _, present := range []string{"Comment", "You can't approve"} {
		if !strings.Contains(rendered, present) {
			test.Errorf("own pull request chooser lacks %q:\n%s", present, rendered)
		}
	}

	other := newReviewer(test)
	other.command("review")
	rendered = other.render()
	for _, present := range []string{"Approve", "Request changes", "Comment"} {
		if !strings.Contains(rendered, present) {
			test.Errorf("chooser on someone else's pull request lacks %q", present)
		}
	}
	if strings.Contains(rendered, "own pull request") {
		test.Error("explanation shown on someone else's pull request")
	}
}

func TestOwnPullRequestChooserStillComments(test *testing.T) {
	composer := newOwnReviewer(test)
	composer.body = "Notes to self."
	composer.command("review")
	composer.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	confirmation, ok := composer.lastConfirm()
	if !ok || confirmation.Text != "Comment on #7 in acme/widgets?" {
		test.Fatalf("confirm = %+v, %v", confirmation, ok)
	}
}

func TestOwnPullRequestRefusesVerdictCommands(test *testing.T) {
	for _, name := range []string{"approve", "request-changes"} {
		test.Run(name, func(test *testing.T) {
			composer := newOwnReviewer(test)
			before := composer.callCount(fake.MethodGetReviewStatus)
			composer.command(name)
			if diff := cmp.Diff([]string{ownPullRequestText}, composer.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if _, asked := composer.lastConfirm(); asked || len(composer.openedOn) != 0 {
				test.Error("a confirmation or editor was opened")
			}
			if composer.callCount(fake.MethodGetReviewStatus) != before || composer.callCount(fake.MethodSubmitReview) != 0 {
				test.Error("a request was sent")
			}
		})
	}
}

func TestSubmitRefusalShowsFriendlySentence(test *testing.T) {
	// The detail says someone else wrote the pull request, but GitHub refuses.
	composer := newReviewer(test)
	composer.client.SetError(fake.MethodSubmitReview, fmt.Errorf("submit review: %w: Review Can not approve your own pull request", github.ErrOwnPullRequest))
	composer.command("approve")
	confirmation, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	composer.run(confirmation.OnYes)
	if got := composer.lastError(); got != ownPullRequestText {
		test.Errorf("error = %q, want %q", got, ownPullRequestText)
	}
}

func TestSubmitUnknownRefusalFallsBackToMessage(test *testing.T) {
	composer := newReviewer(test)
	composer.client.SetError(fake.MethodSubmitReview, errors.New("Review body is too long\x1b[31m"))
	composer.command("approve")
	confirmation, ok := composer.lastConfirm()
	if !ok {
		test.Fatal("no confirmation")
	}
	composer.run(confirmation.OnYes)
	got := composer.lastError()
	if !strings.Contains(got, "Review body is too long") || strings.Contains(got, "\x1b") {
		test.Errorf("error = %q", got)
	}
}
