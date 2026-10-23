package fake

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

// Method names of the inline comment calls, recorded in Call.Method and
// accepted by SetError.
const (
	MethodStartReview         = "StartReview"
	MethodAddReviewThread     = "AddReviewThread"
	MethodUpdateReviewComment = "UpdateReviewComment"
	MethodDeleteReviewComment = "DeleteReviewComment"
	MethodGetReviewComment    = "GetReviewComment"
)

// StartedReview is a recorded successful StartReview call.
type StartedReview struct {
	Repository             github.Repository
	Number                 int
	ReviewIdentifier       string
	CommitObjectIdentifier string
}

// AddedThreads returns the inputs of the successful AddReviewThread calls.
func (c *Client) AddedThreads() []github.AddThreadInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.added)
}

// StartedReviews returns the recorded successful StartReview calls.
func (c *Client) StartedReviews() []StartedReview {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.started)
}

// StartReview implements github.Client. Like GitHub it refuses a second
// pending review, and an unknown pull request ID or an empty commit.
func (c *Client) StartReview(ctx context.Context, in github.StartReviewInput) (github.PendingReview, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodStartReview, Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier}); err != nil {
		return github.PendingReview{}, err
	}
	key := pullRequestKey{in.Repository, in.Number}
	d, ok := c.details[key]
	if !ok {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	if in.PullRequestIdentifier == "" || in.PullRequestIdentifier != d.Identifier {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	}
	if in.CommitObjectIdentifier == "" {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: commit is required", in.Repository, in.Number)
	}
	if _, exists := c.pending[key]; exists {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: user can only have one pending review per pull request", in.Repository, in.Number)
	}
	c.nextIdentifier++
	p := github.PendingReview{Identifier: "fake-pending-review-" + strconv.Itoa(c.nextIdentifier), CommitObjectIdentifier: in.CommitObjectIdentifier}
	c.pending[key] = p
	c.started = append(c.started, StartedReview{Repository: in.Repository, Number: in.Number, ReviewIdentifier: p.Identifier, CommitObjectIdentifier: in.CommitObjectIdentifier})
	return p, nil
}

// AddReviewThread implements github.Client. The thread, with its pending
// comment, joins the pull request's threads and the pending review's count.
// Like GitHub it refuses an empty body and a malformed range; a missing
// pending review yields ErrNotFound.
func (c *Client) AddReviewThread(ctx context.Context, in github.AddThreadInput) (github.ReviewThread, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodAddReviewThread, Repository: in.Repository, Number: in.Number, Path: in.Path, Body: in.Body}); err != nil {
		return github.ReviewThread{}, err
	}
	key := pullRequestKey{in.Repository, in.Number}
	p, ok := c.pending[key]
	if !ok || p.Identifier != in.PendingReviewIdentifier {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	if strings.TrimSpace(in.Body) == "" {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: empty body", in.Repository, in.Number)
	}
	if err := checkRange(in); err != nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, err)
	}
	c.nextIdentifier++
	t := github.ReviewThread{
		Identifier: "fake-thread-" + strconv.Itoa(c.nextIdentifier), SubjectType: github.SubjectLine, Path: in.Path,
		Line: in.Line, OriginalLine: in.Line, DiffSide: in.Side,
		Comments: []github.ReviewComment{{
			Identifier: "fake-review-comment-" + strconv.Itoa(c.nextIdentifier), Author: c.Viewer, Body: in.Body, CreatedAt: c.Now(), Pending: true,
			ViewerCanUpdate: true, ViewerCanDelete: true, ViewerDidAuthor: true,
		}},
	}
	if in.StartLine > 0 {
		t.StartLine, t.OriginalStartLine, t.StartDiffSide = in.StartLine, in.StartLine, in.StartSide
	}
	c.threads[key] = append(slices.Clip(c.threads[key]), t)
	p.CommentCount++
	c.pending[key] = p
	c.added = append(c.added, in)
	return cloneThread(t), nil
}

func checkRange(in github.AddThreadInput) error {
	valid := func(s github.DiffSide) bool { return s == github.SideLeft || s == github.SideRight }
	switch {
	case in.Path == "":
		return fmt.Errorf("path is required")
	case in.Line <= 0 || !valid(in.Side):
		return fmt.Errorf("line %d on side %q is not a valid position", in.Line, in.Side)
	case in.StartLine < 0 || in.StartLine > in.Line:
		return fmt.Errorf("start line %d is after line %d", in.StartLine, in.Line)
	case in.StartLine > 0 && !valid(in.StartSide):
		return fmt.Errorf("start side %q is not valid", in.StartSide)
	}
	return nil
}

// findComment locates a comment among the pull requests' threads.
func (c *Client) findComment(id string) (key pullRequestKey, ti, ci int, ok bool) {
	for k, ts := range c.threads {
		for i, t := range ts {
			for j, cm := range t.Comments {
				if cm.Identifier == id {
					return k, i, j, true
				}
			}
		}
	}
	return pullRequestKey{}, 0, 0, false
}

// findAllowed locates a comment the viewer may change as GitHub reports it
// (the can-flags, so also other people's when the viewer has write access).
// An unknown comment is ErrNotFound, a refused one ErrForbidden.
func (c *Client) findAllowed(id string, allowed func(github.ReviewComment) bool) (key pullRequestKey, ti, ci int, err error) {
	key, ti, ci, ok := c.findComment(id)
	switch {
	case !ok:
		return pullRequestKey{}, 0, 0, fmt.Errorf("comment %s: %w", id, github.ErrNotFound)
	case !allowed(c.threads[key][ti].Comments[ci]):
		return pullRequestKey{}, 0, 0, fmt.Errorf("comment %s: %w", id, github.ErrForbidden)
	}
	return key, ti, ci, nil
}

// publishPending marks the pending comments of a pull request as published,
// as submitting the review does.
func (c *Client) publishPending(key pullRequestKey) {
	ts := cloneThreads(c.threads[key])
	for i := range ts {
		for j := range ts[i].Comments {
			ts[i].Comments[j].Pending = false
		}
	}
	c.threads[key] = ts
}

// UpdateReviewComment implements github.Client. Like GitHub it changes the
// comments the viewer may update (see ViewerCanUpdate), in any state; an empty
// body is refused.
func (c *Client) UpdateReviewComment(ctx context.Context, commentID, body string) (github.ReviewComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodUpdateReviewComment, Body: body}); err != nil {
		return github.ReviewComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return github.ReviewComment{}, fmt.Errorf("update review comment %s: empty body", commentID)
	}
	key, ti, ci, err := c.findAllowed(commentID, func(cm github.ReviewComment) bool { return cm.ViewerCanUpdate })
	if err != nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", err)
	}
	// Fresh slices, so earlier reads are never written to.
	ts := slices.Clone(c.threads[key])
	ts[ti].Comments = slices.Clone(ts[ti].Comments)
	ts[ti].Comments[ci].Body = body
	c.threads[key] = ts
	return cloneReviewComment(ts[ti].Comments[ci]), nil
}

// DeleteReviewComment implements github.Client. Deleting a thread's last
// comment removes the thread, and the pending review's count drops.
func (c *Client) DeleteReviewComment(ctx context.Context, commentID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodDeleteReviewComment}); err != nil {
		return err
	}
	key, ti, ci, err := c.findAllowed(commentID, func(cm github.ReviewComment) bool { return cm.ViewerCanDelete })
	if err != nil {
		return fmt.Errorf("delete review comment: %w", err)
	}
	wasPending := c.threads[key][ti].Comments[ci].Pending
	ts := slices.Clone(c.threads[key])
	ts[ti].Comments = slices.Delete(slices.Clone(ts[ti].Comments), ci, ci+1)
	if len(ts[ti].Comments) == 0 {
		ts = slices.Delete(ts, ti, ti+1)
	}
	c.threads[key] = ts
	if p, ok := c.pending[key]; ok && wasPending {
		p.CommentCount = max(0, p.CommentCount-1)
		c.pending[key] = p
	}
	c.deleted = append(c.deleted, commentID)
	return nil
}

// DeletedComments returns the IDs of the successfully deleted review comments.
func (c *Client) DeletedComments() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.deleted)
}

// GetReviewComment implements github.Client. It finds the comment in any
// pull request's threads; an unknown ID yields ErrNotFound.
func (c *Client) GetReviewComment(ctx context.Context, commentID string) (github.ReviewComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetReviewComment}); err != nil {
		return github.ReviewComment{}, err
	}
	key, ti, ci, ok := c.findComment(commentID)
	if !ok {
		return github.ReviewComment{}, fmt.Errorf("load review comment %s: %w", commentID, github.ErrNotFound)
	}
	return cloneReviewComment(c.threads[key][ti].Comments[ci]), nil
}

// permissions sets what the viewer may do with each comment, as GitHub
// reports it. The viewer's own comments are theirs, with the can-flags as the
// test set them; other people's are not theirs but changeable, as for a
// repository admin (so code must check authorship itself).
func (c *Client) permissions(ts []github.ReviewThread) []github.ReviewThread {
	for i := range ts {
		for j := range ts[i].Comments {
			cm := &ts[i].Comments[j]
			if cm.Author == c.Viewer {
				cm.ViewerDidAuthor = true
				continue
			}
			cm.ViewerDidAuthor, cm.ViewerCanUpdate, cm.ViewerCanDelete = false, true, true
		}
	}
	return ts
}
