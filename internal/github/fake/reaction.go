package fake

import (
	"context"
	"fmt"
	"slices"

	"github.com/MikePors/strigidae/internal/github"
)

// Method names of the reaction calls, recorded in Call.Method and accepted by
// SetError.
const (
	MethodAddReaction    = "AddReaction"
	MethodRemoveReaction = "RemoveReaction"
)

// AddReaction implements github.Client. It changes the reaction summary of a
// review comment: the count goes up and the viewer is marked as having
// reacted. Adding a reaction the viewer already has changes nothing.
func (client *Client) AddReaction(ctx context.Context, subjectIdentifier string, content github.ReactionContent) error {
	return client.react(ctx, MethodAddReaction, subjectIdentifier, content, true)
}

// RemoveReaction implements github.Client. Removing a reaction the viewer does
// not have changes nothing.
func (client *Client) RemoveReaction(ctx context.Context, subjectIdentifier string, content github.ReactionContent) error {
	return client.react(ctx, MethodRemoveReaction, subjectIdentifier, content, false)
}

func (client *Client) react(ctx context.Context, method, subjectIdentifier string, content github.ReactionContent, add bool) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: method, SubjectIdentifier: subjectIdentifier, Reaction: content}); err != nil {
		return err
	}
	key, threadIndex, commentIndex, err := client.findAllowed(subjectIdentifier, func(github.ReviewComment) bool { return true })
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	// Fresh slices, so earlier reads are never written to.
	threads := slices.Clone(client.threads[key])
	threads[threadIndex].Comments = slices.Clone(threads[threadIndex].Comments)
	comment := &threads[threadIndex].Comments[commentIndex]
	comment.Reactions = changeReaction(comment.Reactions, content, add)
	client.threads[key] = threads
	return nil
}

// changeReaction returns reactions with the viewer's reaction of kind content
// added or removed, keeping GitHub's order and dropping a group that reaches
// zero.
func changeReaction(reactions []github.ReactionSummary, content github.ReactionContent, add bool) []github.ReactionSummary {
	out := slices.Clone(reactions)
	index := slices.IndexFunc(out, func(summary github.ReactionSummary) bool { return summary.Content == content })
	switch {
	case add && index < 0:
		out = append(out, github.ReactionSummary{Content: content, Count: 1, ViewerReacted: true})
	case add && !out[index].ViewerReacted:
		out[index].Count++
		out[index].ViewerReacted = true
	case !add && index >= 0 && out[index].ViewerReacted:
		out[index].Count--
		out[index].ViewerReacted = false
		if out[index].Count <= 0 {
			out = slices.Delete(out, index, index+1)
		}
	}
	order := func(summary github.ReactionSummary) int {
		return slices.Index(github.ReactionContents, summary.Content)
	}
	slices.SortStableFunc(out, func(first, second github.ReactionSummary) int { return order(first) - order(second) })
	return out
}
