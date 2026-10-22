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
	Repo            github.Repo
	Number          int
	PullRequestID   string
	Event           github.ReviewEvent
	Body            string
	CommitOID       string
	PendingReviewID string
}

// SetHeadSHA moves the head commit of a seeded pull request, as a push would.
func (c *Client) SetHeadSHA(repo github.Repo, number int, sha string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := prKey{repo, number}
	if d, ok := c.details[key]; ok {
		d.HeadSHA = sha
		c.details[key] = d
	}
}

// SetPendingReview sets the viewer's pending review on a pull request; nil
// removes it, as if it had been submitted or deleted elsewhere.
func (c *Client) SetPendingReview(repo github.Repo, number int, p *github.PendingReview) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p == nil {
		delete(c.pending, prKey{repo, number})
		return
	}
	c.pending[prKey{repo, number}] = *p
}

// Submissions returns the recorded successful SubmitReview calls.
func (c *Client) Submissions() []Submission {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.reviews)
}

// GetReviewStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (c *Client) GetReviewStatus(ctx context.Context, repo github.Repo, number int) (github.ReviewStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetReviewStatus, Repo: repo, Number: number}); err != nil {
		return github.ReviewStatus{}, err
	}
	key := prKey{repo, number}
	d, ok := c.details[key]
	if !ok {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	st := github.ReviewStatus{PullRequestID: d.ID, HeadSHA: d.HeadSHA}
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
	if err := c.begin(ctx, Call{Method: MethodSubmitReview, Repo: in.Repo, Number: in.Number, Body: in.Body}); err != nil {
		return err
	}
	key := prKey{in.Repo, in.Number}
	d, ok := c.details[key]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repo, in.Number, github.ErrNotFound)
	}
	state, ok := reviewStates[in.Event]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: unknown event %q", in.Repo, in.Number, in.Event)
	}
	// The real API identifies the pull request only by its node ID, so the
	// fake refuses an empty or wrong one.
	if in.PullRequestID == "" || in.PullRequestID != d.ID {
		return fmt.Errorf("submit review on %s#%d: unknown pull request ID %q", in.Repo, in.Number, in.PullRequestID)
	}
	var inline int
	if in.PendingReviewID != "" {
		p, ok := c.pending[key]
		if !ok || p.ID != in.PendingReviewID {
			return fmt.Errorf("submit review on %s#%d: %w", in.Repo, in.Number, github.ErrNotFound)
		}
		inline = p.CommentCount
	}
	if in.Event != github.ReviewEventApprove && strings.TrimSpace(in.Body) == "" && inline == 0 {
		return fmt.Errorf("submit review on %s#%d: body is required", in.Repo, in.Number)
	}
	c.publishPending(key)
	delete(c.pending, key)
	c.nextID++
	d.Reviews = append(slices.Clip(d.Reviews), github.Review{
		ID: "fake-review-" + strconv.Itoa(c.nextID), Author: c.Viewer, State: state, Body: in.Body, SubmittedAt: c.Now(),
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
		Repo: in.Repo, Number: in.Number, PullRequestID: in.PullRequestID, Event: in.Event, Body: in.Body,
		CommitOID: in.CommitOID, PendingReviewID: in.PendingReviewID,
	})
	return nil
}

var reviewStates = map[github.ReviewEvent]github.ReviewState{
	github.ReviewEventApprove:        github.ReviewApproved,
	github.ReviewEventRequestChanges: github.ReviewChangesRequested,
	github.ReviewEventComment:        github.ReviewCommented,
}
