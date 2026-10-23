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
func (client *Client) SetMergeSettings(repository github.Repository, settings MergeSettings) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	settings.Allowed = slices.Clone(settings.Allowed)
	client.merge[repository] = settings
}

// Merges returns the recorded successful MergePullRequest calls.
func (client *Client) Merges() []Merge {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.merges)
}

func (client *Client) mergeSettings(repository github.Repository) MergeSettings {
	if settings, ok := client.merge[repository]; ok {
		return settings
	}
	return MergeSettings{Allowed: github.MergeMethods}
}

// GetMergeStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (client *Client) GetMergeStatus(ctx context.Context, repository github.Repository, number int) (github.MergeStatus, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodGetMergeStatus, Repository: repository, Number: number}); err != nil {
		return github.MergeStatus{}, err
	}
	detail, ok := client.details[pullRequestKey{repository, number}]
	if !ok {
		return github.MergeStatus{}, fmt.Errorf("load merge status of %s#%d: %w", repository, number, github.ErrNotFound)
	}
	settings := client.mergeSettings(repository)
	return github.MergeStatus{
		PullRequestIdentifier: detail.Identifier, HeadSHA: detail.HeadSHA, BaseBranch: detail.BaseBranch,
		Allowed: slices.Clone(settings.Allowed), DeleteBranchOnMerge: settings.DeleteBranchOnMerge,
	}, nil
}

// MergePullRequest implements github.Client. Like GitHub it refuses a wrong
// pull request ID, a method the repository does not allow, a head other than
// the expected one, and a pull request that is not open. Inject a refusal
// (failing checks, conflicts) with SetError(MethodMergePullRequest, err).
func (client *Client) MergePullRequest(ctx context.Context, in github.MergeInput) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodMergePullRequest, Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier}); err != nil {
		return err
	}
	key := pullRequestKey{in.Repository, in.Number}
	detail, ok := client.details[key]
	if !ok {
		return fmt.Errorf("merge %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	switch {
	case in.PullRequestIdentifier == "" || in.PullRequestIdentifier != detail.Identifier:
		return fmt.Errorf("merge %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	case !slices.Contains(client.mergeSettings(in.Repository).Allowed, in.Method):
		return fmt.Errorf("merge %s#%d: %s merges are not allowed", in.Repository, in.Number, in.Method)
	case in.ExpectedHeadObjectIdentifier != detail.HeadSHA:
		return fmt.Errorf("merge %s#%d: head branch was modified", in.Repository, in.Number)
	case detail.State != github.PullRequestStateOpen:
		return fmt.Errorf("merge %s#%d: pull request is not open", in.Repository, in.Number)
	}
	detail.State = github.PullRequestStateMerged
	client.details[key] = detail
	client.merges = append(client.merges, Merge{
		Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier, Method: in.Method, ExpectedHeadObjectIdentifier: in.ExpectedHeadObjectIdentifier,
	})
	return nil
}
