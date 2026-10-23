package workspace

import (
	"errors"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// verdicts are the chooser's rows, in order.
var verdicts = []struct {
	label string
	event github.ReviewEvent
}{
	{"Approve", github.ReviewEventApprove},
	{"Request changes", github.ReviewEventRequestChanges},
	{"Comment", github.ReviewEventComment},
}

// commentSavingText is shown when a review or merge is asked for while a
// comment is being saved.
const commentSavingText = "a comment is being saved; try again"

// headMovedText is the notice shown when the review would not apply to the
// commit whose diff is on screen.
const headMovedText = "Head moved since you loaded it; refresh first."

// reviewQuestion is the confirmation text.
func reviewQuestion(event github.ReviewEvent, repo github.Repo, number int, pending *github.PendingReview) string {
	where := fmt.Sprintf("#%d in %s", number, termtext.Clean(repo.String()))
	var q string
	switch event {
	case github.ReviewEventApprove:
		q = "Approve " + where
	case github.ReviewEventRequestChanges:
		q = "Request changes on " + where
	case github.ReviewEventComment:
		q = "Comment on " + where
	}
	if pending != nil && pending.CommentCount > 0 {
		noun := "comments"
		if pending.CommentCount == 1 {
			noun = "comment"
		}
		q += fmt.Sprintf(" with %d pending %s", pending.CommentCount, noun)
	}
	return q + "?"
}

var submittedFmt = map[github.ReviewEvent]string{
	github.ReviewEventApprove:        "Approved #%d",
	github.ReviewEventRequestChanges: "Requested changes on #%d",
	github.ReviewEventComment:        "Commented on #%d",
}

// submittedText is the success message.
func submittedText(event github.ReviewEvent, number int) string {
	return fmt.Sprintf(submittedFmt[event], number)
}

// submitFailure is the error shown when SubmitReview failed. pending says the
// call was for a pending review.
func submitFailure(err error, pending bool) error {
	if pending && errors.Is(err, github.ErrNotFound) {
		return errors.New("your pending review was submitted or deleted elsewhere; refreshing, then try again")
	}
	// The call may or may not have landed, so the view refreshes to show.
	return termtext.SafeError(fmt.Errorf("submit review: %w; refreshed to show the current state", err))
}
