package api

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the inline comment calls; they double as fixture keys.
const (
	opStartReview         = "StartReview"
	opAddReviewThread     = "AddReviewThread"
	opUpdateReviewComment = "UpdateReviewComment"
	opDeleteReviewComment = "DeleteReviewComment"
	opGetReviewComment    = "GetReviewComment"
)

const getReviewCommentQuery = `
query ` + opGetReviewComment + `($id: ID!) {
  node(id: $id) { ... on PullRequestReviewComment { ` + reviewCommentSelection + ` } }
}`

// startReviewMutation creates a pending review: without event, GitHub leaves
// the review in the PENDING state.
const startReviewMutation = `
mutation ` + opStartReview + `($pullRequestId: ID!, $commitOID: GitObjectID!) {
  addPullRequestReview(input: {pullRequestId: $pullRequestId, commitOID: $commitOID}) {
    pullRequestReview { id body }
  }
}`

const addReviewThreadMutation = `
mutation ` + opAddReviewThread + `($reviewId: ID!, $path: String!, $body: String!, $line: Int!, $side: DiffSide!, $startLine: Int, $startSide: DiffSide) {
  addPullRequestReviewThread(input: {pullRequestReviewId: $reviewId, path: $path, body: $body, line: $line, side: $side, startLine: $startLine, startSide: $startSide}) {
    thread { ` + threadSelection + ` }
  }
}`

const updateReviewCommentMutation = `
mutation ` + opUpdateReviewComment + `($commentId: ID!, $body: String!) {
  updatePullRequestReviewComment(input: {pullRequestReviewCommentId: $commentId, body: $body}) {
    pullRequestReviewComment { ` + reviewCommentSelection + ` }
  }
}`

const deleteReviewCommentMutation = `
mutation ` + opDeleteReviewComment + `($commentId: ID!) {
  deletePullRequestReviewComment(input: {id: $commentId}) { clientMutationId }
}`

// StartReview implements github.Client.
func (c *Client) StartReview(ctx context.Context, in github.StartReviewInput) (github.PendingReview, error) {
	var resp struct {
		AddPullRequestReview *struct {
			PullRequestReview *struct{ ID, Body string }
		}
	}
	vars := map[string]any{"pullRequestId": in.PullRequestID, "commitOID": in.CommitOID}
	if err := c.query(ctx, startReviewMutation, vars, &resp); err != nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repo, in.Number, err)
	}
	if resp.AddPullRequestReview == nil || resp.AddPullRequestReview.PullRequestReview == nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repo, in.Number, github.ErrNotFound)
	}
	r := resp.AddPullRequestReview.PullRequestReview
	return github.PendingReview{ID: r.ID, Body: r.Body}, nil
}

// AddReviewThread implements github.Client.
func (c *Client) AddReviewThread(ctx context.Context, in github.AddThreadInput) (github.ReviewThread, error) {
	var resp struct {
		AddPullRequestReviewThread *struct{ Thread *gqlThread }
	}
	vars := map[string]any{
		"reviewId": in.PendingReviewID, "path": in.Path, "body": in.Body,
		"line": in.Line, "side": string(in.Side),
	}
	if in.StartLine > 0 {
		vars["startLine"], vars["startSide"] = in.StartLine, string(in.StartSide)
	}
	if err := c.query(ctx, addReviewThreadMutation, vars, &resp); err != nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repo, in.Number, err)
	}
	if resp.AddPullRequestReviewThread == nil || resp.AddPullRequestReviewThread.Thread == nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repo, in.Number, github.ErrNotFound)
	}
	return mapThread(*resp.AddPullRequestReviewThread.Thread), nil
}

// UpdateReviewComment implements github.Client.
func (c *Client) UpdateReviewComment(ctx context.Context, commentID, body string) (github.ReviewComment, error) {
	var resp struct {
		UpdatePullRequestReviewComment *struct{ PullRequestReviewComment *gqlReviewComment }
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
	var resp struct{ Node *gqlReviewComment }
	if err := c.query(ctx, getReviewCommentQuery, map[string]any{"id": commentID}, &resp); err != nil {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", err)
	}
	// A node of another type matches no fragment and comes back empty.
	if resp.Node == nil || resp.Node.ID == "" {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", github.ErrNotFound)
	}
	return mapReviewComment(*resp.Node), nil
}
