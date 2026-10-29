package workspace

import "github.com/MikePors/strigidae/internal/github"

// commentKind says what a comment target is.
type commentKind int

const (
	// commentReview is a comment in a review thread; the diff pane produces it.
	commentReview commentKind = iota
	// commentTopLevel is a comment on the pull request itself, outside any
	// review. Nothing produces it yet.
	commentTopLevel //nolint:unused // kind for the Overview cursor (#158)
	// commentDescription is the pull request's description. Nothing produces
	// it yet.
	commentDescription //nolint:unused // kind for the Overview cursor (#158)
)

// commentTarget is a comment an action works on, whichever pane it was picked
// in. It carries identifiers and the viewer's rights, so an action that starts
// from it never goes back to the cursor.
type commentTarget struct {
	kind       commentKind
	identifier string
	author     string
	reactions  []github.ReactionSummary
	// The viewer's rights over the comment, as GitHub reported them.
	viewerDidAuthor bool
	viewerCanUpdate bool
	viewerCanDelete bool
	// pending marks a draft in the viewer's pending review.
	pending bool
	// thread is the review thread the comment is in; nil when it is in none.
	thread *github.ReviewThread
}

// reviewCommentTarget makes the target of a comment of a review thread.
func reviewCommentTarget(reviewComment github.ReviewComment, thread github.ReviewThread) commentTarget {
	return commentTarget{
		kind:            commentReview,
		identifier:      reviewComment.Identifier,
		author:          reviewComment.Author,
		reactions:       reviewComment.Reactions,
		viewerDidAuthor: reviewComment.ViewerDidAuthor,
		viewerCanUpdate: reviewComment.ViewerCanUpdate,
		viewerCanDelete: reviewComment.ViewerCanDelete,
		pending:         reviewComment.Pending,
		thread:          &thread,
	}
}
