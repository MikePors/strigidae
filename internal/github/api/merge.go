package api

import (
	"context"
	"fmt"

	"github.com/MikePors/strigidae/internal/github"
)

// Operation names for the merge calls; they double as fixture keys in tests.
const (
	opMergeStatus = "MergeStatus"
	opMerge       = "MergePullRequest"
)

const mergeStatusQuery = `
query ` + opMergeStatus + `($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed deleteBranchOnMerge
    pullRequest(number: $number) { id headRefOid baseRefName }
  }
}`

const mergeMutation = `
mutation ` + opMerge + `($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!, $expectedHeadOid: GitObjectID!) {
  mergePullRequest(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod, expectedHeadOid: $expectedHeadOid}) {
    pullRequest { id state }
  }
}`

// GetMergeStatus implements github.Client.
func (c *Client) GetMergeStatus(ctx context.Context, repo github.Repo, number int) (github.MergeStatus, error) {
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
		PullRequestID: r.PullRequest.ID, HeadSHA: r.PullRequest.HeadRefOid, BaseRef: r.PullRequest.BaseRefName,
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
	// An empty expectedHeadOid would drop GitHub's head check, so none of
	// these may be empty.
	switch {
	case in.PullRequestID == "":
		return fmt.Errorf("merge %s#%d: missing pull request ID", in.Repo, in.Number)
	case in.Method == "":
		return fmt.Errorf("merge %s#%d: missing merge method", in.Repo, in.Number)
	case in.ExpectedHeadOID == "":
		return fmt.Errorf("merge %s#%d: missing expected head commit", in.Repo, in.Number)
	}
	var resp map[string]any
	vars := map[string]any{"pullRequestId": in.PullRequestID, "mergeMethod": string(in.Method), "expectedHeadOid": in.ExpectedHeadOID}
	if err := c.query(ctx, mergeMutation, vars, &resp); err != nil {
		return fmt.Errorf("merge %s#%d: %w", in.Repo, in.Number, err)
	}
	return nil
}
