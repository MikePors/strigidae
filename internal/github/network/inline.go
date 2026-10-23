package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the inline comment calls; they double as fixture keys.
const (
	operationStartReview         = "StartReview"
	operationAddReviewThread     = "AddReviewThread"
	operationUpdateReviewComment = "UpdateReviewComment"
	operationDeleteReviewComment = "DeleteReviewComment"
	operationGetReviewComment    = "GetReviewComment"
)

const getReviewCommentQuery = `
query ` + operationGetReviewComment + `($id: ID!) {
  node(id: $id) { ... on PullRequestReviewComment { ` + reviewCommentSelection + ` } }
}`

// startReviewMutation creates a pending review: without event, GitHub leaves
// the review in the PENDING state.
const startReviewMutation = `
mutation ` + operationStartReview + `($pullRequestId: ID!, $commitOID: GitObjectID!) {
  addPullRequestReview(input: {pullRequestId: $pullRequestId, commitOID: $commitOID}) {
    pullRequestReview { id body }
  }
}`

const addReviewThreadMutation = `
mutation ` + operationAddReviewThread + `($reviewId: ID!, $path: String!, $body: String!, $line: Int!, $side: DiffSide!, $startLine: Int, $startSide: DiffSide) {
  addPullRequestReviewThread(input: {pullRequestReviewId: $reviewId, path: $path, body: $body, line: $line, side: $side, startLine: $startLine, startSide: $startSide}) {
    thread { ` + threadSelection + ` }
  }
}`

const updateReviewCommentMutation = `
mutation ` + operationUpdateReviewComment + `($commentId: ID!, $body: String!) {
  updatePullRequestReviewComment(input: {pullRequestReviewCommentId: $commentId, body: $body}) {
    pullRequestReviewComment { ` + reviewCommentSelection + ` }
  }
}`

const deleteReviewCommentMutation = `
mutation ` + operationDeleteReviewComment + `($commentId: ID!) {
  deletePullRequestReviewComment(input: {id: $commentId}) { clientMutationId }
}`

// StartReview implements github.Client.
func (c *Client) StartReview(ctx context.Context, in github.StartReviewInput) (github.PendingReview, error) {
	var resp struct {
		AddPullRequestReview *struct {
			PullRequestReview *struct{ ID, Body string }
		}
	}
	vars := map[string]any{"pullRequestId": in.PullRequestIdentifier, "commitOID": in.CommitObjectIdentifier}
	if err := c.query(ctx, startReviewMutation, vars, &resp); err != nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, err)
	}
	if resp.AddPullRequestReview == nil || resp.AddPullRequestReview.PullRequestReview == nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	r := resp.AddPullRequestReview.PullRequestReview
	return github.PendingReview{Identifier: r.ID, Body: r.Body}, nil
}

// AddReviewThread implements github.Client.
func (c *Client) AddReviewThread(ctx context.Context, in github.AddThreadInput) (github.ReviewThread, error) {
	var resp struct {
		AddPullRequestReviewThread *struct{ Thread *graphqlThread }
	}
	vars := map[string]any{
		"reviewId": in.PendingReviewIdentifier, "path": in.Path, "body": in.Body,
		"line": in.Line, "side": string(in.Side),
	}
	if in.StartLine > 0 {
		vars["startLine"], vars["startSide"] = in.StartLine, string(in.StartSide)
	}
	if err := c.query(ctx, addReviewThreadMutation, vars, &resp); err != nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, err)
	}
	// GitHub answers a null thread, without errors, when it does not accept the
	// position (as far as known: to be confirmed live).
	if resp.AddPullRequestReviewThread == nil || resp.AddPullRequestReviewThread.Thread == nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w: GitHub did not accept it on %s:%d, which may not be in the diff",
			in.Repository, in.Number, github.ErrCommentRejected, in.Path, in.Line)
	}
	return mapThread(*resp.AddPullRequestReviewThread.Thread), nil
}

// UpdateReviewComment implements github.Client.
func (c *Client) UpdateReviewComment(ctx context.Context, commentID, body string) (github.ReviewComment, error) {
	var resp struct {
		UpdatePullRequestReviewComment *struct{ PullRequestReviewComment *graphqlReviewComment }
	}
	vars := map[string]any{"commentId": commentID, "body": body}
	if err := c.query(ctx, updateReviewCommentMutation, vars, &resp); err != nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", err)
	}
	if resp.UpdatePullRequestReviewComment == nil || resp.UpdatePullRequestReviewComment.PullRequestReviewComment == nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", github.ErrNotFound)
	}
	return mapReviewComment(*resp.UpdatePullRequestReviewComment.PullRequestReviewComment), nil
}

// DeleteReviewComment implements github.Client.
func (c *Client) DeleteReviewComment(ctx context.Context, commentID string) error {
	var resp map[string]any
	if err := c.query(ctx, deleteReviewCommentMutation, map[string]any{"commentId": commentID}, &resp); err != nil {
		return fmt.Errorf("delete review comment: %w", err)
	}
	return nil
}

// GetReviewComment implements github.Client.
func (c *Client) GetReviewComment(ctx context.Context, commentID string) (github.ReviewComment, error) {
	var resp struct{ Node *graphqlReviewComment }
	if err := c.query(ctx, getReviewCommentQuery, map[string]any{"id": commentID}, &resp); err != nil {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", err)
	}
	// A node of another type matches no fragment and comes back empty.
	if resp.Node == nil || resp.Node.ID == "" {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", github.ErrNotFound)
	}
	return mapReviewComment(*resp.Node), nil
}
