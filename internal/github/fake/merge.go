package fake

import (
	"context"
	"fmt"
	"slices"

	"github.com/MikePors/strigidae/internal/github"
)

// MergeSettings is a repository's merge configuration.
type MergeSettings struct {
	// Allowed lists the merge methods the repository allows.
	Allowed             []github.MergeMethod
	DeleteBranchOnMerge bool
}

// Merge is a recorded successful MergePullRequest call.
type Merge struct {
	Repository                   github.Repository
	Number                       int
	PullRequestIdentifier        string
	Method                       github.MergeMethod
	ExpectedHeadObjectIdentifier string
}

// SetMergeSettings sets the merge settings of repo. Without it every method
// is allowed and head branches are not deleted automatically.
func (c *Client) SetMergeSettings(repo github.Repository, s MergeSettings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.Allowed = slices.Clone(s.Allowed)
	c.merge[repo] = s
}

// Merges returns the recorded successful MergePullRequest calls.
func (c *Client) Merges() []Merge {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.merges)
}

func (c *Client) mergeSettings(repo github.Repository) MergeSettings {
	if s, ok := c.merge[repo]; ok {
		return s
	}
	return MergeSettings{Allowed: github.MergeMethods}
}

// GetMergeStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (c *Client) GetMergeStatus(ctx context.Context, repo github.Repository, number int) (github.MergeStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetMergeStatus, Repository: repo, Number: number}); err != nil {
		return github.MergeStatus{}, err
	}
	d, ok := c.details[pullRequestKey{repo, number}]
	if !ok {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	s := c.mergeSettings(repo)
	return github.MergeStatus{
		PullRequestIdentifier: d.Identifier, HeadSHA: d.HeadSHA, BaseBranch: d.BaseBranch,
		Allowed: slices.Clone(s.Allowed), DeleteBranchOnMerge: s.DeleteBranchOnMerge,
	}, nil
}

// MergePullRequest implements github.Client. Like GitHub it refuses a wrong
// pull request ID, a method the repository does not allow, a head other than
// the expected one, and a pull request that is not open. Inject a refusal
// (failing checks, conflicts) with SetError(MethodMergePullRequest, err).
func (c *Client) MergePullRequest(ctx context.Context, in github.MergeInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodMergePullRequest, Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier}); err != nil {
		return err
	}
	key := pullRequestKey{in.Repository, in.Number}
	d, ok := c.details[key]
	if !ok {
		return fmt.Errorf("merge %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	switch {
	case in.PullRequestIdentifier == "" || in.PullRequestIdentifier != d.Identifier:
		return fmt.Errorf("merge %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	case !slices.Contains(c.mergeSettings(in.Repository).Allowed, in.Method):
		return fmt.Errorf("merge %s#%d: %s merges are not allowed", in.Repository, in.Number, in.Method)
	case in.ExpectedHeadObjectIdentifier != d.HeadSHA:
		return fmt.Errorf("merge %s#%d: head branch was modified", in.Repository, in.Number)
	case d.State != github.PullRequestStateOpen:
		return fmt.Errorf("merge %s#%d: pull request is not open", in.Repository, in.Number)
	}
	d.State = github.PullRequestStateMerged
	c.details[key] = d
	c.merges = append(c.merges, Merge{
		Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier, Method: in.Method, ExpectedHeadObjectIdentifier: in.ExpectedHeadObjectIdentifier,
	})
	return nil
}
