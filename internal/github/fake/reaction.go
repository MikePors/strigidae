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
	comment.Reactions = github.WithViewerReaction(comment.Reactions, content, add)
	client.threads[key] = threads
	return nil
}
