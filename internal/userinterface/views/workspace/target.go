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
	// ends in " to " + words; the lines one is "move onto a diff line to " +
	// words.
	words string
	// rule narrows a comment-level target to the comments the action may use;
	// nil allows every comment.
	rule *commentRule
}

// commentRule says which comments an action may use and what it says about
// the others.
type commentRule struct {
	// mine reports whether the comment is the viewer's.
	mine func(github.ReviewComment) bool
	// allowed reports whether the viewer may use their own comment.
	allowed func(github.ReviewComment) bool
	// notMine is said when the cursor is not on one of the viewer's comments.
	notMine string
	// cannot is said when it is the viewer's comment but not allowed.
	cannot string
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
// returns the command that tells why there is none. Every level needs the
// diff pane on screen, focused or not.
func (view *View) resolveTarget(request request) (resolvedTarget, tea.Cmd) {
	if hidden := view.diffHidden(request.words); hidden != nil {
		return resolvedTarget{}, hidden
	}
	var context diffview.Context
	if view.selection() > 0 { // the Overview has no diff cursor
		context = view.filePane.Context()
	}
	switch request.level {
	case levelLines:
		if !context.HasLines {
			return resolvedTarget{}, userinterface.Info("move onto a diff line to " + request.words)
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
	rule := request.rule
	switch {
	case !context.OnComment:
		if rule != nil {
			return resolvedTarget{}, userinterface.Info(rule.notMine)
		}
		return resolvedTarget{}, userinterface.Info("move onto a comment to " + request.words)
	case rule != nil && !rule.mine(context.Threads[0].Comments[context.CommentIndex]):
		return resolvedTarget{}, userinterface.Info(rule.notMine)
	case rule != nil && !rule.allowed(context.Threads[0].Comments[context.CommentIndex]):
		return resolvedTarget{}, userinterface.Info(rule.cannot)
	}
	return resolvedTarget{thread: context.Threads[0], commentIndex: context.CommentIndex}, nil
}

// diffHidden returns the notice to show when the diff pane is not on screen (a
// narrow terminal showing the Files pane), or nil. The cursor of a pane the
// user cannot see is not something to act on.
func (view *View) diffHidden(words string) tea.Cmd {
	if _, rightWidth := view.paneWidths(); rightWidth == 0 {
		return userinterface.Info("focus the diff to " + words)
	}
	return nil
}
