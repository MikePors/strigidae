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
func (client *Client) SetHeadSHA(repository github.Repository, number int, sha string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	key := pullRequestKey{repository, number}
	if detail, ok := client.details[key]; ok {
		detail.HeadSHA = sha
		client.details[key] = detail
	}
}

// SetPendingReview sets the viewer's pending review on a pull request; nil
// removes it, as if it had been submitted or deleted elsewhere.
func (client *Client) SetPendingReview(repository github.Repository, number int, pendingReview *github.PendingReview) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if pendingReview == nil {
		delete(client.pending, pullRequestKey{repository, number})
		return
	}
	client.pending[pullRequestKey{repository, number}] = *pendingReview
}

// Submissions returns the recorded successful SubmitReview calls.
func (client *Client) Submissions() []Submission {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.reviews)
}

// GetReviewStatus implements github.Client. It answers ErrNotFound for a pull
// request the fake has no detail for.
func (client *Client) GetReviewStatus(ctx context.Context, repository github.Repository, number int) (github.ReviewStatus, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodGetReviewStatus, Repository: repository, Number: number}); err != nil {
		return github.ReviewStatus{}, err
	}
	key := pullRequestKey{repository, number}
	detail, ok := client.details[key]
	if !ok {
		return github.ReviewStatus{}, fmt.Errorf("load review status of %s#%d: %w", repository, number, github.ErrNotFound)
	}
	status := github.ReviewStatus{PullRequestIdentifier: detail.Identifier, HeadSHA: detail.HeadSHA}
	if pendingReview, ok := client.pending[key]; ok {
		status.Pending = &pendingReview
	}
	return status, nil
}

// SubmitReview implements github.Client. Like GitHub it refuses an unknown
// event, and an empty body for a verdict other than approval unless a pending
// review brings inline comments. Also like GitHub, it refuses an approval or a
// request for changes on a pull request whose detail has ViewerDidAuthor, with
// ErrOwnPullRequest. A submitted pending review disappears, and
// the review joins the detail's reviews.
func (client *Client) SubmitReview(ctx context.Context, in github.SubmitReviewInput) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodSubmitReview, Repository: in.Repository, Number: in.Number, Body: in.Body}); err != nil {
		return err
	}
	key := pullRequestKey{in.Repository, in.Number}
	detail, ok := client.details[key]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	state, ok := reviewStates[in.Event]
	if !ok {
		return fmt.Errorf("submit review on %s#%d: unknown event %q", in.Repository, in.Number, in.Event)
	}
	// The real API identifies the pull request only by its node ID, so the
	// fake refuses an empty or wrong one.
	if in.PullRequestIdentifier == "" || in.PullRequestIdentifier != detail.Identifier {
		return fmt.Errorf("submit review on %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	}
	if detail.ViewerDidAuthor && in.Event != github.ReviewEventComment {
		return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, github.ErrOwnPullRequest)
	}
	var inline int
	if in.PendingReviewIdentifier != "" {
		pendingReview, ok := client.pending[key]
		if !ok || pendingReview.Identifier != in.PendingReviewIdentifier {
			return fmt.Errorf("submit review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
		}
		inline = pendingReview.CommentCount
	}
	if in.Event != github.ReviewEventApprove && strings.TrimSpace(in.Body) == "" && inline == 0 {
		return fmt.Errorf("submit review on %s#%d: body is required", in.Repository, in.Number)
	}
	client.publishPending(key)
	delete(client.pending, key)
	client.nextIdentifier++
	detail.Reviews = append(slices.Clip(detail.Reviews), github.Review{
		Identifier: "fake-review-" + strconv.Itoa(client.nextIdentifier), Author: client.Viewer, State: state, Body: in.Body, SubmittedAt: client.Now(),
	})
	switch in.Event {
	case github.ReviewEventApprove:
		detail.ReviewDecision = github.ReviewDecisionApproved
	case github.ReviewEventRequestChanges:
		detail.ReviewDecision = github.ReviewDecisionChangesRequested
	case github.ReviewEventComment:
	}
	client.details[key] = detail
	client.reviews = append(client.reviews, Submission{
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
