package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// operationUpdatePullRequest doubles as the fixture key in tests.
const operationUpdatePullRequest = "UpdatePullRequest"

const updatePullRequestMutation = `
mutation ` + operationUpdatePullRequest + `($pullRequestId: ID!, $title: String!, $body: String!) {
  updatePullRequest(input: {pullRequestId: $pullRequestId, title: $title, body: $body}) {
    pullRequest { id title }
  }
}`

// UpdatePullRequest implements github.Client.
func (client *Client) UpdatePullRequest(ctx context.Context, in github.UpdatePullRequestInput) error {
	var response map[string]any
	variables := map[string]any{"pullRequestId": in.PullRequestIdentifier, "title": in.Title, "body": in.Body}
	if err := client.query(ctx, updatePullRequestMutation, variables, &response); err != nil {
		return fmt.Errorf("update %s#%d: %w", in.Repository, in.Number, err)
	}
	return nil
}
