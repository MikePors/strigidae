package api

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the review calls; they double as fixture keys in tests.
const (
	opReviewStatus = "ReviewStatus"
	opAddReview    = "AddReview"
	opSubmitReview = "SubmitReview"
)

const reviewStatusQuery = `
query ` + opReviewStatus + `($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id headRefOid
      reviews(first: 10, states: PENDING) { nodes { id body commit { oid } comments(first: 100) { totalCount nodes { id } } } }
    }
  }
}`

// addReviewMutation creates a review and submits it in one call, because
// event is given.
const addReviewMutation = `
mutation ` + opAddReview + `($pullRequestId: ID!, $event: PullRequestReviewEvent!, $body: String!, $commitOID: GitObjectID!) {
  addPullRequestReview(input: {pullRequestId: $pullRequestId, event: $event, body: $body, commitOID: $commitOID}) {
    pullRequestReview { id state }
  }
}`

const submitReviewMutation = `
mutation ` + opSubmitReview + `($reviewId: ID!, $event: PullRequestReviewEvent!, $body: String!) {
  submitPullRequestReview(input: {pullRequestReviewId: $reviewId, event: $event, body: $body}) {
    pullRequestReview { id state }
  }
}`

// GetReviewStatus implements github.Client.
func (c *Client) GetReviewStatus(ctx context.Context, repo github.Repo, number int) (github.ReviewStatus, error) {
	var resp struct {
		Repository *struct {
			PullRequest *struct {
				ID         string
				HeadRefOid string
				Reviews    struct {
					Nodes []struct {
						ID       string
						Body     string
						Commit   *struct{ Oid string }
						Comments struct {
							TotalCount int
							Nodes      []struct{ ID string }
						}
					}
				}
			}
		}
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	if err := c.query(ctx, reviewStatusQuery, vars, &resp); err != nil {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repo, number, err)
	}
	if resp.Repository == nil || resp.Repository.PullRequest == nil {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	pr := resp.Repository.PullRequest
	st := github.ReviewStatus{PullRequestID: pr.ID, HeadSHA: pr.HeadRefOid}
	// Pending reviews are visible only to their author, so any node is the viewer's.
	if len(pr.Reviews.Nodes) > 0 {
		n := pr.Reviews.Nodes[0]
		st.Pending = &github.PendingReview{ID: n.ID, Body: n.Body, CommentCount: n.Comments.TotalCount}
		for _, c := range n.Comments.Nodes {
			st.Pending.CommentIDs = append(st.Pending.CommentIDs, c.ID)
		}
		if n.Commit != nil {
			st.Pending.CommitOID = n.Commit.Oid
		}
	}
	return st, nil
}

// SubmitReview implements github.Client.
func (c *Client) SubmitReview(ctx context.Context, in github.SubmitReviewInput) error {
	var resp map[string]any
	var err error
	if in.PendingReviewID != "" {
		vars := map[string]any{"reviewId": in.PendingReviewID, "event": string(in.Event), "body": in.Body}
		err = c.query(ctx, submitReviewMutation, vars, &resp)
	} else {
		vars := map[string]any{"pullRequestId": in.PullRequestID, "event": string(in.Event), "body": in.Body, "commitOID": in.CommitOID}
		err = c.query(ctx, addReviewMutation, vars, &resp)
	}
	if err != nil {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repo, in.Number, err)
	}
	return nil
}
