package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

const addReactionMutation = `
mutation ` + operationAddReaction + `($subjectId: ID!, $content: ReactionContent!) {
  addReaction(input: {subjectId: $subjectId, content: $content}) { reaction { content } }
}`

const removeReactionMutation = `
mutation ` + operationRemoveReaction + `($subjectId: ID!, $content: ReactionContent!) {
  removeReaction(input: {subjectId: $subjectId, content: $content}) { reaction { content } }
}`

// AddReaction implements github.Client.
func (client *Client) AddReaction(ctx context.Context, subjectIdentifier string, content github.ReactionContent) error {
	var response struct {
		AddReaction *struct{ Reaction *struct{ Content string } }
	}
	variables := map[string]any{"subjectId": subjectIdentifier, "content": string(content)}
	if err := client.query(ctx, addReactionMutation, variables, &response); err != nil {
		return fmt.Errorf("add reaction: %w", err)
	}
	if response.AddReaction == nil {
		return fmt.Errorf("add reaction: %w", github.ErrNotFound)
	}
	return nil
}

// RemoveReaction implements github.Client.
func (client *Client) RemoveReaction(ctx context.Context, subjectIdentifier string, content github.ReactionContent) error {
	var response struct {
		RemoveReaction *struct{ Reaction *struct{ Content string } }
	}
	variables := map[string]any{"subjectId": subjectIdentifier, "content": string(content)}
	if err := client.query(ctx, removeReactionMutation, variables, &response); err != nil {
		return fmt.Errorf("remove reaction: %w", err)
	}
	if response.RemoveReaction == nil {
		return fmt.Errorf("remove reaction: %w", github.ErrNotFound)
	}
	return nil
}
