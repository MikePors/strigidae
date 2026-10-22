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
	Repo            github.Repo
	Number          int
	PullRequestID   string
	Method          github.MergeMethod
	ExpectedHeadOID string
}

// SetMergeSettings sets the merge settings of repo. Without it every method
// is allowed and head branches are not deleted automatically.
func (c *Client) SetMergeSettings(repo github.Repo, s MergeSettings) {
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

func (c *Client) mergeSettings(repo github.Repo) MergeSettings {
	if s, ok := c.merge[repo]; ok {
		return s
	}
	return MergeSettings{Allowed: github.MergeMethods}
}

// GetMergeStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (c *Client) GetMergeStatus(ctx context.Context, repo github.Repo, number int) (github.MergeStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetMergeStatus, Repo: repo, Number: number}); err != nil {
		return github.MergeStatus{}, err
	}
	d, ok := c.details[prKey{repo, number}]
	if !ok {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	s := c.mergeSettings(repo)
	return github.MergeStatus{
		PullRequestID: d.ID, HeadSHA: d.HeadSHA, BaseRef: d.BaseRef,
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
	if err := c.begin(ctx, Call{Method: MethodMergePullRequest, Repo: in.Repo, Number: in.Number, PullRequestID: in.PullRequestID}); err != nil {
		return err
	}
	key := prKey{in.Repo, in.Number}
	d, ok := c.details[key]
	if !ok {
		return fmt.Errorf("merge %s#%d: %w", in.Repo, in.Number, github.ErrNotFound)
	}
	switch {
	case in.ExpectedHeadOID == "" || in.Method == "":
		return fmt.Errorf("merge %s#%d: missing merge method or expected head", in.Repo, in.Number)
	case in.PullRequestID == "" || in.PullRequestID != d.ID:
		return fmt.Errorf("merge %s#%d: unknown pull request ID %q", in.Repo, in.Number, in.PullRequestID)
	case !slices.Contains(c.mergeSettings(in.Repo).Allowed, in.Method):
		return fmt.Errorf("merge %s#%d: %s merges are not allowed", in.Repo, in.Number, in.Method)
	case in.ExpectedHeadOID != d.HeadSHA:
		return fmt.Errorf("merge %s#%d: head branch was modified", in.Repo, in.Number)
	case d.State != github.PRStateOpen:
		return fmt.Errorf("merge %s#%d: pull request is not open", in.Repo, in.Number)
	}
	d.State = github.PRStateMerged
	c.details[key] = d
	c.merges = append(c.merges, Merge{
		Repo: in.Repo, Number: in.Number, PullRequestID: in.PullRequestID, Method: in.Method, ExpectedHeadOID: in.ExpectedHeadOID,
	})
	return nil
}
