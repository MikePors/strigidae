package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

func (inliner *inliner) discardCalls() []string { return inliner.methods(fake.MethodDeleteReview) }

// askDiscard runs :discard-review and returns the question asked.
func (inliner *inliner) askDiscard() userinterface.ConfirmMessage {
	inliner.test.Helper()
	inliner.messages = nil
	inliner.command("discard-review")
	confirmation, ok := inliner.lastConfirm()
	if !ok {
		inliner.test.Fatalf("no question asked; messages %v", inliner.messages)
	}
	return confirmation
}

func TestDiscardReviewAsksThenDeletes(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("first draft")
	confirmation := inliner.askDiscard()
	if want := "Discard your pending review and its 1 draft comment?"; confirmation.Text != want {
		test.Errorf("question = %q, want %q", confirmation.Text, want)
	}
	if len(inliner.discardCalls()) != 0 || inliner.view.writes != 0 {
		test.Fatalf("written or busy before the answer: %v, writes %d", inliner.discardCalls(), inliner.view.writes)
	}
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{"Pending review discarded"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(inliner.client.DeletedReviews()) != 1 || len(inliner.discardCalls()) != 1 {
		test.Errorf("deleted %v", inliner.client.DeletedReviews())
	}
	status, err := inliner.client.GetReviewStatus(test.Context(), testRepository, 7)
	if err != nil || status.Pending != nil {
		test.Errorf("pending review remains: %+v, %v", status.Pending, err)
	}
	for _, thread := range inliner.view.threads {
		for _, comment := range thread.Comments {
			if comment.Pending {
				test.Errorf("draft still shown: %+v", thread)
			}
		}
	}
	if inliner.view.writes != 0 {
		test.Errorf("writes in flight: %d", inliner.view.writes)
	}
}

func TestDiscardReviewQuestionNamesTheCount(test *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, "Discard your pending review?"},
		{1, "Discard your pending review and its 1 draft comment?"},
		{3, "Discard your pending review and its 3 draft comments?"},
	}
	for _, testCase := range tests {
		test.Run(testCase.want, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "r1", CommentCount: testCase.count})
			if got := inliner.askDiscard().Text; got != testCase.want {
				test.Errorf("question = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestDiscardReviewDeclinedWritesNothing(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("draft")
	inliner.askDiscard() // declining is the shell dropping OnYes
	if len(inliner.discardCalls()) != 0 || len(inliner.client.DeletedReviews()) != 0 || inliner.view.writes != 0 {
		test.Errorf("deleted %v or busy (writes %d) after the question", inliner.client.DeletedReviews(), inliner.view.writes)
	}
	status, err := inliner.client.GetReviewStatus(test.Context(), testRepository, 7)
	if err != nil || status.Pending == nil || status.Pending.CommentCount != 1 {
		test.Errorf("the pending review did not survive: %+v, %v", status.Pending, err)
	}
	inliner.askDiscard() // the view is free to ask again
}

func TestDiscardReviewWithNothingPending(test *testing.T) {
	inliner := newInliner(test)
	inliner.messages = nil
	inliner.command("discard-review")
	if _, asked := inliner.lastConfirm(); asked {
		test.Error("asked with nothing to discard")
	}
	if diff := cmp.Diff([]string{"no pending review"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(inliner.discardCalls()) != 0 || inliner.view.writes != 0 {
		test.Errorf("calls %v, writes %d", inliner.discardCalls(), inliner.view.writes)
	}
}

func TestDiscardReviewChangedBetweenAskingAndYes(test *testing.T) {
	test.Run("gone", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.addPending("draft")
		confirmation := inliner.askDiscard()
		inliner.submitElsewhere() // the drafts are published on github.com
		inliner.messages = nil
		inliner.run(confirmation.OnYes)
		for _, thread := range inliner.view.threads {
			for _, comment := range thread.Comments {
				if comment.Pending {
					test.Errorf("the screen still shows a pending draft: %+v", thread)
				}
			}
		}
		if len(inliner.discardCalls()) != 0 {
			test.Error("deleted on a stale yes")
		}
		if diff := cmp.Diff([]string{"no pending review"}, inliner.infoMessages()); diff != "" {
			test.Errorf("info (-want +got):\n%s", diff)
		}
	})
	test.Run("another review", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "r1", CommentCount: 2})
		confirmation := inliner.askDiscard()
		inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "r2", CommentCount: 2})
		inliner.messages = nil
		inliner.run(confirmation.OnYes)
		if len(inliner.discardCalls()) != 0 {
			test.Fatal("deleted a review that was not the one asked about")
		}
		again, asked := inliner.lastConfirm()
		if !asked || again.Text != "Discard your pending review and its 2 draft comments?" {
			test.Fatalf("not asked again: %v, %+v", asked, again)
		}
		inliner.messages = nil
		inliner.run(again.OnYes)
		if diff := cmp.Diff([]string{"r2"}, inliner.client.DeletedReviews()); diff != "" {
			test.Errorf("deleted (-want +got):\n%s", diff)
		}
	})
	test.Run("more drafts", func(test *testing.T) {
		inliner := newInliner(test)
		inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "r1", CommentCount: 1})
		confirmation := inliner.askDiscard()
		inliner.client.SetPendingReview(testRepository, 7, &github.PendingReview{Identifier: "r1", CommentCount: 4})
		inliner.messages = nil
		inliner.run(confirmation.OnYes)
		again, asked := inliner.lastConfirm()
		if len(inliner.discardCalls()) != 0 || !asked || !strings.Contains(again.Text, "4 draft") {
			test.Errorf("calls %v, asked %v, %q", inliner.discardCalls(), asked, again.Text)
		}
	})
}

func TestDiscardReviewRefusedWhileBusy(test *testing.T) {
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
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.addPending("draft")
			testCase.busy(inliner)
			writes := inliner.view.writes
			inliner.messages = nil
			inliner.command("discard-review")
			if diff := cmp.Diff([]string{testCase.want}, inliner.infoMessages()); diff != "" {
				test.Errorf("info (-want +got):\n%s", diff)
			}
			if _, asked := inliner.lastConfirm(); asked || inliner.view.writes != writes {
				test.Errorf("asked %v, writes %d", asked, inliner.view.writes)
			}
		})
	}
}

func TestDiscardReviewYesIsRefusedWhileAMergeRuns(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("draft")
	confirmation := inliner.askDiscard()
	inliner.view.merge.merging = true
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{"a merge is in progress"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if len(inliner.discardCalls()) != 0 || inliner.view.writes != 0 {
		test.Errorf("calls %v, writes %d", inliner.discardCalls(), inliner.view.writes)
	}
}

func TestDiscardReviewGoneAtTheWriteIsANotice(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("draft")
	confirmation := inliner.askDiscard()
	inliner.client.SetError(fake.MethodDeleteReview, fmt.Errorf("delete review: %w", github.ErrNotFound))
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if diff := cmp.Diff([]string{"no pending review"}, inliner.infoMessages()); diff != "" {
		test.Errorf("info (-want +got):\n%s", diff)
	}
	if message := inliner.lastError(); message != "" {
		test.Errorf("error shown: %q", message)
	}
	if inliner.view.writes != 0 {
		test.Errorf("view stayed busy: %d", inliner.view.writes)
	}
}

func TestDiscardReviewDeleteErrorIsShown(test *testing.T) {
	inliner := newInliner(test)
	inliner.addPending("draft")
	confirmation := inliner.askDiscard()
	inliner.client.SetError(fake.MethodDeleteReview, errors.New("boom"))
	inliner.messages = nil
	inliner.run(confirmation.OnYes)
	if !strings.Contains(inliner.lastError(), "discard review") || !strings.Contains(inliner.lastError(), "boom") {
		test.Errorf("error = %q", inliner.lastError())
	}
	if inliner.view.writes != 0 {
		test.Errorf("view stayed busy: %d", inliner.view.writes)
	}
	if len(inliner.client.DeletedReviews()) != 0 {
		test.Error("deleted despite the error")
	}
}

func TestDiscardReviewTimesOut(test *testing.T) {
	tests := []struct {
		name         string
		blockRead    bool
		wantMayApply bool
	}{
		{"re-read", true, false},
		{"delete", false, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner := newInliner(test)
			inliner.addPending("first draft")
			inliner.view.timeouts = shortTimeouts
			if testCase.blockRead {
				inliner.view.client = blockingReadClient{blockingClient{Client: inliner.client}}
			} else {
				inliner.view.client = blockingClient{Client: inliner.client}
			}
			inliner.messages = nil
			if testCase.blockRead {
				inliner.command("discard-review") // the first re-read, before any question
			} else {
				inliner.run(inliner.askDiscard().OnYes)
			}
			err := inliner.lastErr()
			if !errors.Is(err, context.DeadlineExceeded) {
				test.Fatalf("error = %v, want a deadline error", err)
			}
			if got := strings.Contains(err.Error(), "it may have gone through"); got != testCase.wantMayApply {
				test.Errorf("error = %v, says may have gone through = %v, want %v", err, got, testCase.wantMayApply)
			}
			if inliner.view.writes != 0 {
				test.Errorf("writes in flight = %d, want 0", inliner.view.writes)
			}
		})
	}
}

// blockingReadClient also blocks the review state read.
type blockingReadClient struct{ blockingClient }

func (client blockingReadClient) GetReviewStatus(ctx context.Context, _ github.Repository, _ int) (github.ReviewStatus, error) {
	return github.ReviewStatus{}, client.wait(ctx)
}
