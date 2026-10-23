package fake

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

// Submission is a recorded successful SubmitReview call.
type Submission struct {
	Repository              github.Repository
	Number                  int
	PullRequestIdentifier   string
	Event                   github.ReviewEvent
	Body                    string
	CommitObjectIdentifier  string
	PendingReviewIdentifier string
}

// SetHeadSHA moves the head commit of a seeded pull request, as a push would.
func (c *Client) SetHeadSHA(repo github.Repository, number int, sha string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := pullRequestKey{repo, number}
	if d, ok := c.details[key]; ok {
		d.HeadSHA = sha
		c.details[key] = d
	}
}

// SetPendingReview sets the viewer's pending review on a pull request; nil
// removes it, as if it had been submitted or deleted elsewhere.
func (c *Client) SetPendingReview(repo github.Repository, number int, p *github.PendingReview) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p == nil {
		delete(c.pending, pullRequestKey{repo, number})
		return
	}
	c.pending[pullRequestKey{repo, number}] = *p
}

// Submissions returns the recorded successful SubmitReview calls.
func (c *Client) Submissions() []Submission {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.reviews)
}

// GetReviewStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (c *Client) GetReviewStatus(ctx context.Context, repo github.Repository, number int) (github.ReviewStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetReviewStatus, Repository: repo, Number: number}); err != nil {
		return github.ReviewStatus{}, err
	}
	key := pullRequestKey{repo, number}
	d, ok := c.details[key]
	if !ok {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	st := github.ReviewStatus{PullRequestIdentifier: d.Identifier, HeadSHA: d.HeadSHA}
	if p, ok := c.pending[key]; ok {
		st.Pending = &p
	}
	return st, nil
}

// SubmitReview implements github.Client. Like GitHub it refuses an unknown
// event, and an empty body for a verdict other than approval unless a pending
// review brings inline comments. A submitted pending review disappears, and
// the review joins the detail's reviews.
func (c *Client) SubmitReview(ctx context.Context, in github.SubmitReviewInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodSubmitReview, Repository: in.Repository, Number: in.Number, Body: in.Body}); err != nil {
		return err
	}
	key := pullRequestKey{in.Repository, in.Number}
	d, ok := c.details[key]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	state, ok := reviewStates[in.Event]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: unknown event %q", in.Repository, in.Number, in.Event)
	}
	// The real API identifies the pull request only by its node ID, so the
	// fake refuses an empty or wrong one.
	if in.PullRequestIdentifier == "" || in.PullRequestIdentifier != d.Identifier {
		return fmt.Errorf("submit review on %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	}
	var inline int
	if in.PendingReviewIdentifier != "" {
		p, ok := c.pending[key]
		if !ok || p.Identifier != in.PendingReviewIdentifier {
			return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
		}
		inline = p.CommentCount
	}
	if in.Event != github.ReviewEventApprove && strings.TrimSpace(in.Body) == "" && inline == 0 {
		return fmt.Errorf("submit review on %s#%d: body is required", in.Repository, in.Number)
	}
	c.publishPending(key)
	delete(c.pending, key)
	c.nextIdentifier++
	d.Reviews = append(slices.Clip(d.Reviews), github.Review{
		Identifier: "fake-review-" + strconv.Itoa(c.nextIdentifier), Author: c.Viewer, State: state, Body: in.Body, SubmittedAt: c.Now(),
	})
	switch in.Event {
	case github.ReviewEventApprove:
		d.ReviewDecision = github.ReviewDecisionApproved
	case github.ReviewEventRequestChanges:
		d.ReviewDecision = github.ReviewDecisionChangesRequested
	case github.ReviewEventComment:
	}
	c.details[key] = d
	c.reviews = append(c.reviews, Submission{
		Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier, Event: in.Event, Body: in.Body,
		CommitObjectIdentifier: in.CommitObjectIdentifier, PendingReviewIdentifier: in.PendingReviewIdentifier,
	})
	return nil
}

var reviewStates = map[github.ReviewEvent]github.ReviewState{
	github.ReviewEventApprove:        github.ReviewApproved,
	github.ReviewEventRequestChanges: github.ReviewChangesRequested,
	github.ReviewEventComment:        github.ReviewCommented,
}
