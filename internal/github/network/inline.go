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
	operationDeleteReview        = "DeleteReview"
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

const deleteReviewMutation = `
mutation ` + operationDeleteReview + `($reviewId: ID!) {
  deletePullRequestReview(input: {pullRequestReviewId: $reviewId}) { clientMutationId }
}`

// StartReview implements github.Client.
func (client *Client) StartReview(ctx context.Context, in github.StartReviewInput) (github.PendingReview, error) {
	var response struct {
		AddPullRequestReview *struct {
			PullRequestReview *struct{ ID, Body string }
		}
	}
	variables := map[string]any{"pullRequestId": in.PullRequestIdentifier, "commitOID": in.CommitObjectIdentifier}
	if err := client.query(ctx, startReviewMutation, variables, &response); err != nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, err)
	}
	if response.AddPullRequestReview == nil || response.AddPullRequestReview.PullRequestReview == nil {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	record := response.AddPullRequestReview.PullRequestReview
	return github.PendingReview{Identifier: record.ID, Body: record.Body}, nil
}

// AddReviewThread implements github.Client.
func (client *Client) AddReviewThread(ctx context.Context, in github.AddThreadInput) (github.ReviewThread, error) {
	var response struct {
		AddPullRequestReviewThread *struct{ Thread *graphqlThread }
	}
	variables := map[string]any{
		"reviewId": in.PendingReviewIdentifier, "path": in.Path, "body": in.Body,
		"line": in.Line, "side": string(in.Side),
	}
	if in.StartLine > 0 {
		variables["startLine"], variables["startSide"] = in.StartLine, string(in.StartSide)
	}
	if err := client.query(ctx, addReviewThreadMutation, variables, &response); err != nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, err)
	}
	// GitHub answers a null thread, without errors, when it does not accept the
	// position (as far as known: to be confirmed live).
	if response.AddPullRequestReviewThread == nil || response.AddPullRequestReviewThread.Thread == nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w: GitHub did not accept it on %s:%d, which may not be in the diff",
			in.Repository, in.Number, github.ErrCommentRejected, in.Path, in.Line)
	}
	return mapThread(*response.AddPullRequestReviewThread.Thread), nil
}

// UpdateReviewComment implements github.Client.
func (client *Client) UpdateReviewComment(ctx context.Context, commentIdentifier, body string) (github.ReviewComment, error) {
	var response struct {
		UpdatePullRequestReviewComment *struct{ PullRequestReviewComment *graphqlReviewComment }
	}
	variables := map[string]any{"commentId": commentIdentifier, "body": body}
	if err := client.query(ctx, updateReviewCommentMutation, variables, &response); err != nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", err)
	}
	if response.UpdatePullRequestReviewComment == nil || response.UpdatePullRequestReviewComment.PullRequestReviewComment == nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", github.ErrNotFound)
	}
	return mapReviewComment(*response.UpdatePullRequestReviewComment.PullRequestReviewComment), nil
}

// DeleteReviewComment implements github.Client.
func (client *Client) DeleteReviewComment(ctx context.Context, commentIdentifier string) error {
	var response map[string]any
	if err := client.query(ctx, deleteReviewCommentMutation, map[string]any{"commentId": commentIdentifier}, &response); err != nil {
		return fmt.Errorf("delete review comment: %w", err)
	}
	return nil
}

// DeleteReview implements github.Client.
func (client *Client) DeleteReview(ctx context.Context, reviewIdentifier string) error {
	var response map[string]any
	if err := client.query(ctx, deleteReviewMutation, map[string]any{"reviewId": reviewIdentifier}, &response); err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	return nil
}

// GetReviewComment implements github.Client.
func (client *Client) GetReviewComment(ctx context.Context, commentIdentifier string) (github.ReviewComment, error) {
	var response struct{ Node *graphqlReviewComment }
	if err := client.query(ctx, getReviewCommentQuery, map[string]any{"id": commentIdentifier}, &response); err != nil {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", err)
	}
	// A node of another type matches no fragment and comes back empty.
	if response.Node == nil || response.Node.ID == "" {
		return github.ReviewComment{}, fmt.Errorf("load review comment: %w", github.ErrNotFound)
	}
	return mapReviewComment(*response.Node), nil
}
