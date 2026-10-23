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
func (c *Client) GetMergeStatus(ctx context.Context, repo github.Repository, number int) (github.MergeStatus, error) {
	var resp struct {
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
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	if err := c.query(ctx, mergeStatusQuery, vars, &resp); err != nil {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repo, number, err)
	}
	r := resp.Repository
	if r == nil || r.PullRequest == nil {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	st := github.MergeStatus{
		PullRequestIdentifier: r.PullRequest.ID, HeadSHA: r.PullRequest.HeadRefOid, BaseBranch: r.PullRequest.BaseRefName,
		DeleteBranchOnMerge: r.DeleteBranchOnMerge,
	}
	for _, m := range []struct {
		method  github.MergeMethod
		allowed bool
	}{
		{github.MergeMethodMerge, r.MergeCommitAllowed},
		{github.MergeMethodSquash, r.SquashMergeAllowed},
		{github.MergeMethodRebase, r.RebaseMergeAllowed},
	} {
		if m.allowed {
			st.Allowed = append(st.Allowed, m.method)
		}
	}
	return st, nil
}

// MergePullRequest implements github.Client.
func (c *Client) MergePullRequest(ctx context.Context, in github.MergeInput) error {
	var resp map[string]any
	vars := map[string]any{"pullRequestId": in.PullRequestIdentifier, "mergeMethod": string(in.Method), "expectedHeadOid": in.ExpectedHeadObjectIdentifier}
	if err := c.query(ctx, mergeMutation, vars, &resp); err != nil {
		return fmt.Errorf("merge %s#%d: %w", in.Repository, in.Number, err)
	}
	return nil
}
