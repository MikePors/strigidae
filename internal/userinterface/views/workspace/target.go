package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

// level is the kind of thing an action works on, from the diff pane's cursor
// context outwards.
type level int

const (
	levelLines   level = iota // the line or selection a new comment goes on
	levelThread               // the comment thread under the cursor
	levelComment              // one comment of an open thread
)

// request is what an action asks the resolver for.
type request struct {
	level level
	// words finish the notices: "reply", "resolve it", "react to it". The
	// focus notice is "focus the diff to " + words; the several-threads one
	// ends in " to " + words.
	words string
	// allow narrows a comment-level target to the comments the action may use;
	// nil allows every comment.
	allow func(github.ReviewComment) bool
}

// resolvedTarget is what the cursor context gives the action. Only the field
// of the requested level is set; a comment-level target also sets thread.
type resolvedTarget struct {
	lines        diffview.Range
	thread       github.ReviewThread
	commentIndex int
}

// comment is the comment of a comment-level target.
func (resolved resolvedTarget) comment() github.ReviewComment {
	return resolved.thread.Comments[resolved.commentIndex]
}

// resolveTarget turns the diff cursor into the target the request names, or
// returns the command that tells why there is none. Thread and comment
// targets need the diff pane focused; lines do not.
func (view *View) resolveTarget(request request) (resolvedTarget, tea.Cmd) {
	if request.level != levelLines && view.focus != paneRight {
		return resolvedTarget{}, userinterface.Info("focus the diff to " + request.words)
	}
	var context diffview.Context
	if view.selection() > 0 { // the Overview has no diff cursor
		context = view.filePane.Context()
	}
	switch request.level {
	case levelLines:
		if !context.HasLines {
			return resolvedTarget{}, userinterface.Info("move onto a diff line to comment on it")
		}
		return resolvedTarget{lines: context.Lines}, nil
	case levelThread:
		switch len(context.Threads) {
		case 0:
			return resolvedTarget{}, userinterface.Info("no comment thread here")
		case 1:
			return resolvedTarget{thread: context.Threads[0]}, nil
		}
		return resolvedTarget{}, userinterface.Info("several threads on this line: move onto one to " + request.words)
	}
	if context.OnComment && (request.allow == nil || request.allow(context.Threads[0].Comments[context.CommentIndex])) {
		return resolvedTarget{thread: context.Threads[0], commentIndex: context.CommentIndex}, nil
	}
	if request.allow != nil {
		return resolvedTarget{}, userinterface.Info("move onto a comment of yours to " + request.words)
	}
	return resolvedTarget{}, userinterface.Info("move onto a comment to " + request.words)
}
