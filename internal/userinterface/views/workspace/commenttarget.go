package workspace

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// commentKind says what a comment target is.
type commentKind int

const (
	// commentReview is a comment in a review thread; the diff pane produces it.
	commentReview commentKind = iota
	// commentTopLevel is a comment on the pull request itself, outside any
	// review; the Overview target picker produces it.
	commentTopLevel
	// commentDescription is the pull request's description.
	commentDescription
)

// commentTarget is a comment an action works on, whichever pane it was picked
// in. It carries identifiers and the viewer's rights, so an action that starts
// from it never goes back to the cursor.
type commentTarget struct {
	kind       commentKind
	identifier string
	author     string
	body       string
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

// resolveCommentTarget refreshes an explicit comment context from the loaded
// pull request and review threads.
func (view *View) resolveCommentTarget(identifier string) (commentTarget, bool) {
	if detail := view.detail; detail != nil {
		if detail.Identifier == identifier {
			return commentTarget{kind: commentDescription, identifier: identifier, author: detail.Author, body: detail.Body, reactions: detail.Reactions}, true
		}
		for _, issueComment := range detail.Comments {
			if issueComment.Identifier == identifier {
				return commentTarget{kind: commentTopLevel, identifier: identifier, author: issueComment.Author, body: issueComment.Body, reactions: issueComment.Reactions}, true
			}
		}
	}
	for _, reviewThread := range view.threads {
		for _, reviewComment := range reviewThread.Comments {
			if reviewComment.Identifier == identifier {
				return reviewCommentTarget(reviewComment, reviewThread), true
			}
		}
	}
	return commentTarget{}, false
}

// commentSelection holds an action until its missing comment context is chosen.
type commentSelection struct {
	action  actionEntry
	call    invocation
	targets []commentTarget
}

func (view *View) openCommentTargetPicker(chosen actionEntry, call invocation) {
	targets := view.overviewCommentTargets()
	items := make([]string, len(targets))
	for i, target := range targets {
		items[i] = commentTargetLabel(target, pickerLabelWidth(view.width))
	}
	view.pickerComment = &commentSelection{action: chosen, call: call, targets: targets}
	view.pickerReaction, view.pickerChecks = nil, nil
	view.picker = picker.New(view.theme, chosen.commentPickerTitle, items)
}

func (view *View) overviewCommentTargets() []commentTarget {
	if view.detail == nil {
		return nil
	}
	detail := view.detail
	out := make([]commentTarget, 0, len(detail.Comments)+1)
	out = append(out, commentTarget{kind: commentDescription, identifier: detail.Identifier, author: detail.Author, body: detail.Body, reactions: detail.Reactions})
	for _, issueComment := range detail.Comments {
		out = append(out, commentTarget{kind: commentTopLevel, identifier: issueComment.Identifier, author: issueComment.Author, body: issueComment.Body, reactions: issueComment.Reactions})
	}
	return out
}

func commentTargetLabel(target commentTarget, width int) string {
	kind := "Comment"
	if target.kind == commentDescription {
		kind = "Description"
	}
	body := strings.Join(strings.Fields(terminaltext.Clean(target.body)), " ")
	prefix := kind + " · " + comment.Author(target.author) + " · "
	return ansi.Truncate(prefix+body, max(width, 0), "…")
}
