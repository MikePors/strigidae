package fake

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

// PullRequestUpdates returns the inputs of the successful UpdatePullRequest calls.
func (client *Client) PullRequestUpdates() []github.UpdatePullRequestInput {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.updates)
}

// UpdatePullRequest implements github.Client. Like GitHub it refuses a wrong
// pull request ID, an empty title, and a viewer who may not update the pull
// request (see PullRequestDetail.ViewerCanUpdate).
func (client *Client) UpdatePullRequest(ctx context.Context, in github.UpdatePullRequestInput) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodUpdatePullRequest, Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier, Title: in.Title, Body: in.Body}); err != nil {
		return err
	}
	key := pullRequestKey{in.Repository, in.Number}
	detail, ok := client.details[key]
	switch {
	case !ok:
		return fmt.Errorf("update %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	case in.PullRequestIdentifier == "" || in.PullRequestIdentifier != detail.Identifier:
		return fmt.Errorf("update %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	case !detail.ViewerCanUpdate:
		return fmt.Errorf("update %s#%d: %w", in.Repository, in.Number, github.ErrForbidden)
	case strings.TrimSpace(in.Title) == "":
		return fmt.Errorf("update %s#%d: empty title", in.Repository, in.Number)
	}
	detail.Title, detail.Body = in.Title, in.Body
	client.details[key] = detail
	client.updates = append(client.updates, in)
	return nil
}
