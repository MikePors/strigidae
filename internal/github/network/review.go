package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the review calls; they double as fixture keys in tests.
const (
	operationReviewStatus = "ReviewStatus"
	operationAddReview    = "AddReview"
	operationSubmitReview = "SubmitReview"
)

const reviewStatusQuery = `
query ` + operationReviewStatus + `($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id headRefOid
      reviews(first: 10, states: PENDING) { nodes { id body commit { oid } comments { totalCount } } }
    }
  }
}`

// addReviewMutation creates a review and submits it in one call, because
// event is given.
const addReviewMutation = `
mutation ` + operationAddReview + `($pullRequestId: ID!, $event: PullRequestReviewEvent!, $body: String!, $commitOID: GitObjectID!) {
  addPullRequestReview(input: {pullRequestId: $pullRequestId, event: $event, body: $body, commitOID: $commitOID}) {
    pullRequestReview { id state }
  }
}`

const submitReviewMutation = `
mutation ` + operationSubmitReview + `($reviewId: ID!, $event: PullRequestReviewEvent!, $body: String!) {
  submitPullRequestReview(input: {pullRequestReviewId: $reviewId, event: $event, body: $body}) {
    pullRequestReview { id state }
  }
}`

// GetReviewStatus implements github.Client.
func (client *Client) GetReviewStatus(ctx context.Context, repository github.Repository, number int) (github.ReviewStatus, error) {
	var response struct {
		Repository *struct {
			PullRequest *struct {
				ID         string
				HeadRefOid string
				Reviews    struct {
					Nodes []struct {
						ID       string
						Body     string
						Commit   *struct{ Oid string }
						Comments struct{ TotalCount int }
					}
				}
			}
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "number": number}
	if err := client.query(ctx, reviewStatusQuery, variables, &response); err != nil {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repository, number, err)
	}
	if response.Repository == nil || response.Repository.PullRequest == nil {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repository, number, github.ErrNotFound)
	}
	pullRequest := response.Repository.PullRequest
	status := github.ReviewStatus{PullRequestIdentifier: pullRequest.ID, HeadSHA: pullRequest.HeadRefOid}
	// Pending reviews are visible only to their author, so any node is the viewer's.
	if len(pullRequest.Reviews.Nodes) > 0 {
		node := pullRequest.Reviews.Nodes[0]
		status.Pending = &github.PendingReview{Identifier: node.ID, Body: node.Body, CommentCount: node.Comments.TotalCount}
		if node.Commit != nil {
			status.Pending.CommitObjectIdentifier = node.Commit.Oid
		}
	}
	return status, nil
}

// SubmitReview implements github.Client.
func (client *Client) SubmitReview(ctx context.Context, in github.SubmitReviewInput) error {
	var response map[string]any
	var err error
	if in.PendingReviewIdentifier != "" {
		variables := map[string]any{"reviewId": in.PendingReviewIdentifier, "event": string(in.Event), "body": in.Body}
		err = client.query(ctx, submitReviewMutation, variables, &response)
	} else {
		variables := map[string]any{"pullRequestId": in.PullRequestIdentifier, "event": string(in.Event), "body": in.Body, "commitOID": in.CommitObjectIdentifier}
		err = client.query(ctx, addReviewMutation, variables, &response)
	}
	if err != nil {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, err)
	}
	return nil
}
