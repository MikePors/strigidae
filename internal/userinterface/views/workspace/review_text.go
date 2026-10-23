package workspace

import (
	"errors"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
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
func reviewQuestion(event github.ReviewEvent, repository github.Repository, number int, pending *github.PendingReview) string {
	where := fmt.Sprintf("#%d in %s", number, terminaltext.Clean(repository.String()))
	var query string
	switch event {
	case github.ReviewEventApprove:
		query = "Approve " + where
	case github.ReviewEventRequestChanges:
		query = "Request changes on " + where
	case github.ReviewEventComment:
		query = "Comment on " + where
	}
	if pending != nil && pending.CommentCount > 0 {
		noun := "comments"
		if pending.CommentCount == 1 {
			noun = "comment"
		}
		query += fmt.Sprintf(" with %d pending %s", pending.CommentCount, noun)
	}
	return query + "?"
}

var submittedFormat = map[github.ReviewEvent]string{
	github.ReviewEventApprove:        "Approved #%d",
	github.ReviewEventRequestChanges: "Requested changes on #%d",
	github.ReviewEventComment:        "Commented on #%d",
}

// submittedText is the success message.
func submittedText(event github.ReviewEvent, number int) string {
	return fmt.Sprintf(submittedFormat[event], number)
}

// submitFailure is the error shown when SubmitReview failed. pending says the
// call was for a pending review.
func submitFailure(err error, pending bool) error {
	if pending && errors.Is(err, github.ErrNotFound) {
		return errors.New("your pending review was submitted or deleted elsewhere; refreshing, then try again")
	}
	// The call may or may not have landed, so the view refreshes to show.
	return terminaltext.SafeError(fmt.Errorf("submit review: %w; refreshed to show the current state", err))
}
