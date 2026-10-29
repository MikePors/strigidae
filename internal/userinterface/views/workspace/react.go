package workspace

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/comment"
	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// A reaction is confirmed by choosing it, in the picker or by naming it in
// the command, so :react never asks y/N. Choosing toggles: a reaction the
// viewer has is removed, otherwise it is added.

// reactionTarget is the comment :react was started on, as the screen showed it.
type reactionTarget struct {
	commentIdentifier string
	reactions         []github.ReactionSummary
}

// summary returns the comment's summary for one kind of reaction (zero if none).
func (target reactionTarget) summary(content github.ReactionContent) github.ReactionSummary {
	for _, summary := range target.reactions {
		if summary.Content == content {
			return summary
		}
	}
	return github.ReactionSummary{Content: content}
}

// reactionToggledMessage is the result of adding or removing a reaction.
type reactionToggledMessage struct {
	origin            origin
	commentIdentifier string
	content           github.ReactionContent
	removed           bool
	err               error
}

// reactCommand handles :react [name].
func (view *View) reactCommand(name string) tea.Cmd {
	switch {
	case view.review.submitting:
		return userinterface.Info("a review is being submitted")
	case view.merge.merging:
		return userinterface.Info("a merge is in progress")
	case view.writes > 0:
		return userinterface.Info(commentSavingText)
	}
	var content github.ReactionContent
	if name != "" {
		index := slices.IndexFunc(github.ReactionContents, func(candidate github.ReactionContent) bool { return candidate.Shortcode() == name })
		if index < 0 {
			return userinterface.Error(fmt.Errorf("not a reaction: %s (use %s)", terminaltext.Clean(name), reactionNames()))
		}
		content = github.ReactionContents[index]
	}
	resolved, command := view.resolveTarget(request{level: levelComment, words: "react to it"})
	if command != nil {
		return command
	}
	reviewComment := resolved.comment()
	target := reactionTarget{commentIdentifier: reviewComment.Identifier, reactions: reviewComment.Reactions}
	if content != "" {
		return view.toggleReaction(target, content)
	}
	if view.picker != nil || view.review.chooser != nil {
		return nil
	}
	view.openReactionPicker(target, reviewComment.Author)
	return nil
}

func reactionNames() string {
	names := make([]string, len(github.ReactionContents))
	for i, content := range github.ReactionContents {
		names[i] = content.Shortcode()
	}
	return strings.Join(names, ", ")
}

func (view *View) openReactionPicker(target reactionTarget, author string) {
	items := make([]string, len(github.ReactionContents))
	for i, content := range github.ReactionContents {
		items[i] = reactionLabel(content, target.summary(content))
	}
	view.pickerReaction = &target
	view.picker = picker.New(view.theme, "React to "+comment.Author(author)+"'s comment", items)
}

// reactionLabel is the plain text of a picker row: emoji, shortcode, the count
// and a mark when the viewer has reacted.
func reactionLabel(content github.ReactionContent, summary github.ReactionSummary) string {
	count, mark := "", ""
	if summary.Count > 0 {
		count = strconv.Itoa(summary.Count)
	}
	if summary.ViewerReacted {
		mark = "✓"
	}
	return strings.TrimRight(fmt.Sprintf("%s %-8s %2s  %s", content.Emoji(), content.Shortcode(), count, mark), " ")
}

// toggleReaction removes the viewer's reaction of the given kind, or adds it.
func (view *View) toggleReaction(target reactionTarget, content github.ReactionContent) tea.Cmd {
	removed := target.summary(content).ViewerReacted
	origin, client, timeouts := view.origin(), view.client, view.timeouts
	view.writeStarted()
	return func() tea.Msg {
		ctx, cancel := timeouts.RequestContext()
		defer cancel()
		var err error
		if removed {
			err = client.RemoveReaction(ctx, target.commentIdentifier, content)
		} else {
			err = client.AddReaction(ctx, target.commentIdentifier, content)
		}
		err = userinterface.WriteError(err)
		result := reactionToggledMessage{origin: origin, commentIdentifier: target.commentIdentifier, content: content, removed: removed, err: err}
		if err == nil {
			return result
		}
		return withShownError(result, commentFailure(err, "react"))
	}
}

// reactionToggled shows the outcome and reloads the threads so the counts under
// the comment update.
func (view *View) reactionToggled(message reactionToggledMessage) tea.Cmd {
	view.writeDone()
	if message.err != nil {
		return nil // the error is already shown
	}
	// The comment shows the change at once, so a second :react decides from
	// what the first did; the reload then brings GitHub's own counts.
	view.applyReaction(message)
	text := "reacted " + message.content.Emoji()
	if message.removed {
		text = "removed " + message.content.Emoji()
	}
	return tea.Batch(userinterface.Info(text), view.reloadThreads())
}

// applyReaction records a successful write in the comment on screen.
func (view *View) applyReaction(message reactionToggledMessage) {
	// Fresh slices, so the ones handed to the file pane earlier are never written to.
	threads := slices.Clone(view.threads)
	for i := range threads {
		index := slices.IndexFunc(threads[i].Comments, func(reviewComment github.ReviewComment) bool {
			return reviewComment.Identifier == message.commentIdentifier
		})
		if index < 0 {
			continue
		}
		threads[i].Comments = slices.Clone(threads[i].Comments)
		reviewComment := &threads[i].Comments[index]
		reviewComment.Reactions = github.WithViewerReaction(reviewComment.Reactions, message.content, !message.removed)
		view.threads = threads
		view.filePane.SetThreads(view.threads)
		view.overview.invalidate()
		view.layout()
		return
	}
}
