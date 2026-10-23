package network

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the merge calls; they double as fixture keys in tests.
const (
	operationMergeStatus = "MergeStatus"
	operationMerge       = "MergePullRequest"
)

const mergeStatusQuery = `
query ` + operationMergeStatus + `($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed deleteBranchOnMerge
    pullRequest(number: $number) { id headRefOid baseRefName }
  }
}`

const mergeMutation = `
mutation ` + operationMerge + `($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!, $expectedHeadOid: GitObjectID!) {
  mergePullRequest(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod, expectedHeadOid: $expectedHeadOid}) {
    pullRequest { id state }
  }
}`

// GetMergeStatus implements github.Client.
func (client *Client) GetMergeStatus(ctx context.Context, repository github.Repository, number int) (github.MergeStatus, error) {
	var response struct {
		Repository *struct {
			MergeCommitAllowed  bool
			SquashMergeAllowed  bool
			RebaseMergeAllowed  bool
			DeleteBranchOnMerge bool
			PullRequest         *struct {
				ID          string
				HeadRefOid  string
				BaseRefName string
			}
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "number": number}
	if err := client.query(ctx, mergeStatusQuery, variables, &response); err != nil {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repository, number, err)
	}
	record := response.Repository
	if record == nil || record.PullRequest == nil {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repository, number, github.ErrNotFound)
	}
	status := github.MergeStatus{
		PullRequestIdentifier: record.PullRequest.ID, HeadSHA: record.PullRequest.HeadRefOid, BaseBranch: record.PullRequest.BaseRefName,
		DeleteBranchOnMerge: record.DeleteBranchOnMerge,
	}
	for _, method := range []struct {
		method  github.MergeMethod
		allowed bool
	}{
		{github.MergeMethodMerge, record.MergeCommitAllowed},
		{github.MergeMethodSquash, record.SquashMergeAllowed},
		{github.MergeMethodRebase, record.RebaseMergeAllowed},
	} {
		if method.allowed {
			status.Allowed = append(status.Allowed, method.method)
		}
	}
	return status, nil
}

// MergePullRequest implements github.Client.
func (client *Client) MergePullRequest(ctx context.Context, in github.MergeInput) error {
	var response map[string]any
	variables := map[string]any{"pullRequestId": in.PullRequestIdentifier, "mergeMethod": string(in.Method), "expectedHeadOid": in.ExpectedHeadObjectIdentifier}
	if err := client.query(ctx, mergeMutation, variables, &response); err != nil {
		return fmt.Errorf("merge %s#%d: %w", in.Repository, in.Number, err)
	}
	return nil
}
