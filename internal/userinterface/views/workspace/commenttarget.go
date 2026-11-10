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

// Overview targets intentionally omit viewer rights: edit/delete support must
// populate those rights before enabling actions with ownership rules.
func descriptionTarget(detail github.PullRequestDetail) commentTarget {
	return commentTarget{kind: commentDescription, identifier: detail.Identifier, author: detail.Author, body: detail.Body, reactions: detail.Reactions}
}

func topLevelCommentTarget(comment github.IssueComment) commentTarget {
	return commentTarget{kind: commentTopLevel, identifier: comment.Identifier, author: comment.Author, body: comment.Body, reactions: comment.Reactions}
}

// resolveCommentTarget refreshes an explicit comment context from the loaded
// pull request and review threads.
func (view *View) resolveCommentTarget(identifier string) (commentTarget, bool) {
	if detail := view.detail; detail != nil {
		if detail.Identifier == identifier {
			return descriptionTarget(*detail), true
		}
		for _, issueComment := range detail.Comments {
			if issueComment.Identifier == identifier {
				return topLevelCommentTarget(issueComment), true
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
	targets := make([]commentTarget, 0, len(detail.Comments)+1)
	targets = append(targets, descriptionTarget(*detail))
	for _, issueComment := range detail.Comments {
		targets = append(targets, topLevelCommentTarget(issueComment))
	}
	return targets
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
