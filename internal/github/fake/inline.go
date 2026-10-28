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
	MethodDeleteReview        = "DeleteReview"
)

// StartedReview is a recorded successful StartReview call.
type StartedReview struct {
	Repository             github.Repository
	Number                 int
	ReviewIdentifier       string
	CommitObjectIdentifier string
}

// AddedThreads returns the inputs of the successful AddReviewThread calls.
func (client *Client) AddedThreads() []github.AddThreadInput {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.added)
}

// StartedReviews returns the recorded successful StartReview calls.
func (client *Client) StartedReviews() []StartedReview {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.started)
}

// StartReview implements github.Client. Like GitHub it refuses a second
// pending review, and an unknown pull request ID or an empty commit.
func (client *Client) StartReview(ctx context.Context, in github.StartReviewInput) (github.PendingReview, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodStartReview, Repository: in.Repository, Number: in.Number, PullRequestIdentifier: in.PullRequestIdentifier}); err != nil {
		return github.PendingReview{}, err
	}
	key := pullRequestKey{in.Repository, in.Number}
	detail, ok := client.details[key]
	if !ok {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	if in.PullRequestIdentifier == "" || in.PullRequestIdentifier != detail.Identifier {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: unknown pull request ID %q", in.Repository, in.Number, in.PullRequestIdentifier)
	}
	if in.CommitObjectIdentifier == "" {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: commit is required", in.Repository, in.Number)
	}
	if _, exists := client.pending[key]; exists {
		return github.PendingReview{}, fmt.Errorf("start review on %s#%d: user can only have one pending review per pull request", in.Repository, in.Number)
	}
	client.nextIdentifier++
	pendingReview := github.PendingReview{Identifier: "fake-pending-review-" + strconv.Itoa(client.nextIdentifier), CommitObjectIdentifier: in.CommitObjectIdentifier}
	client.pending[key] = pendingReview
	client.started = append(client.started, StartedReview{Repository: in.Repository, Number: in.Number, ReviewIdentifier: pendingReview.Identifier, CommitObjectIdentifier: in.CommitObjectIdentifier})
	return pendingReview, nil
}

// AddReviewThread implements github.Client. The thread, with its pending
// comment, joins the pull request's threads and the pending review's count.
// Like GitHub it refuses an empty body and a malformed range; a missing
// pending review yields ErrNotFound.
func (client *Client) AddReviewThread(ctx context.Context, in github.AddThreadInput) (github.ReviewThread, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodAddReviewThread, Repository: in.Repository, Number: in.Number, Path: in.Path, Body: in.Body}); err != nil {
		return github.ReviewThread{}, err
	}
	key := pullRequestKey{in.Repository, in.Number}
	pendingReview, ok := client.pending[key]
	if !ok || pendingReview.Identifier != in.PendingReviewIdentifier {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, github.ErrNotFound)
	}
	if strings.TrimSpace(in.Body) == "" {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: empty body", in.Repository, in.Number)
	}
	if err := checkRange(in); err != nil {
		return github.ReviewThread{}, fmt.Errorf("add review comment on %s#%d: %w", in.Repository, in.Number, err)
	}
	client.nextIdentifier++
	thread := github.ReviewThread{
		Identifier: "fake-thread-" + strconv.Itoa(client.nextIdentifier), SubjectType: github.SubjectLine, Path: in.Path,
		Line: in.Line, OriginalLine: in.Line, DiffSide: in.Side,
		Comments: []github.ReviewComment{{
			Identifier: "fake-review-comment-" + strconv.Itoa(client.nextIdentifier), Author: client.Viewer, Body: in.Body, CreatedAt: client.Now(), Pending: true,
			ViewerCanUpdate: true, ViewerCanDelete: true, ViewerDidAuthor: true,
		}},
	}
	if in.StartLine > 0 {
		thread.StartLine, thread.OriginalStartLine, thread.StartDiffSide = in.StartLine, in.StartLine, in.StartSide
	}
	client.threads[key] = append(slices.Clip(client.threads[key]), thread)
	pendingReview.CommentCount++
	client.pending[key] = pendingReview
	client.added = append(client.added, in)
	return cloneThread(thread), nil
}

func checkRange(in github.AddThreadInput) error {
	valid := func(side github.DiffSide) bool { return side == github.SideLeft || side == github.SideRight }
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
func (client *Client) findComment(identifier string) (key pullRequestKey, threadIndex, commentIndex int, ok bool) {
	for key, threads := range client.threads {
		for i, thread := range threads {
			for j, comment := range thread.Comments {
				if comment.Identifier == identifier {
					return key, i, j, true
				}
			}
		}
	}
	return pullRequestKey{}, 0, 0, false
}

// findAllowed locates a comment the viewer may change as GitHub reports it
// (the can-flags, so also other people's when the viewer has write access).
// An unknown comment is ErrNotFound, a refused one ErrForbidden.
func (client *Client) findAllowed(identifier string, allowed func(github.ReviewComment) bool) (key pullRequestKey, threadIndex, commentIndex int, err error) {
	key, threadIndex, commentIndex, ok := client.findComment(identifier)
	switch {
	case !ok:
		return pullRequestKey{}, 0, 0, fmt.Errorf("comment %s: %w", identifier, github.ErrNotFound)
	case !allowed(client.threads[key][threadIndex].Comments[commentIndex]):
		return pullRequestKey{}, 0, 0, fmt.Errorf("comment %s: %w", identifier, github.ErrForbidden)
	}
	return key, threadIndex, commentIndex, nil
}

// publishPending marks the pending comments of a pull request as published,
// as submitting the review does.
func (client *Client) publishPending(key pullRequestKey) {
	threads := cloneThreads(client.threads[key])
	for i := range threads {
		for j := range threads[i].Comments {
			threads[i].Comments[j].Pending = false
		}
	}
	client.threads[key] = threads
}

// UpdateReviewComment implements github.Client. Like GitHub it changes the
// comments the viewer may update (see ViewerCanUpdate), in any state; an empty
// body is refused.
func (client *Client) UpdateReviewComment(ctx context.Context, commentIdentifier, body string) (github.ReviewComment, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodUpdateReviewComment, Body: body}); err != nil {
		return github.ReviewComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return github.ReviewComment{}, fmt.Errorf("update review comment %s: empty body", commentIdentifier)
	}
	key, threadIndex, commentIndex, err := client.findAllowed(commentIdentifier, func(comment github.ReviewComment) bool { return comment.ViewerCanUpdate })
	if err != nil {
		return github.ReviewComment{}, fmt.Errorf("update review comment: %w", err)
	}
	// Fresh slices, so earlier reads are never written to.
	threads := slices.Clone(client.threads[key])
	threads[threadIndex].Comments = slices.Clone(threads[threadIndex].Comments)
	threads[threadIndex].Comments[commentIndex].Body = body
	client.threads[key] = threads
	return cloneReviewComment(threads[threadIndex].Comments[commentIndex]), nil
}

// DeleteReviewComment implements github.Client. Deleting a thread's last
// comment removes the thread, and the pending review's count drops.
func (client *Client) DeleteReviewComment(ctx context.Context, commentIdentifier string) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodDeleteReviewComment}); err != nil {
		return err
	}
	key, threadIndex, commentIndex, err := client.findAllowed(commentIdentifier, func(comment github.ReviewComment) bool { return comment.ViewerCanDelete })
	if err != nil {
		return fmt.Errorf("delete review comment: %w", err)
	}
	wasPending := client.threads[key][threadIndex].Comments[commentIndex].Pending
	threads := slices.Clone(client.threads[key])
	threads[threadIndex].Comments = slices.Delete(slices.Clone(threads[threadIndex].Comments), commentIndex, commentIndex+1)
	if len(threads[threadIndex].Comments) == 0 {
		threads = slices.Delete(threads, threadIndex, threadIndex+1)
	}
	client.threads[key] = threads
	if pendingReview, ok := client.pending[key]; ok && wasPending {
		pendingReview.CommentCount = max(0, pendingReview.CommentCount-1)
		client.pending[key] = pendingReview
	}
	client.deleted = append(client.deleted, commentIdentifier)
	return nil
}

// DeletedComments returns the IDs of the successfully deleted review comments.
func (client *Client) DeletedComments() []string {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.deleted)
}

// DeleteReview implements github.Client. Like GitHub it deletes the pending
// review with its draft comments, and a thread left without comments goes with
// them. Any identifier that is not a pending review, a submitted review
// included, yields ErrNotFound; GitHub may report a submitted one differently.
func (client *Client) DeleteReview(ctx context.Context, reviewIdentifier string) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodDeleteReview, ReviewIdentifier: reviewIdentifier}); err != nil {
		return err
	}
	for key, pendingReview := range client.pending {
		if pendingReview.Identifier != reviewIdentifier {
			continue
		}
		delete(client.pending, key)
		var kept []github.ReviewThread
		for _, thread := range client.threads[key] {
			thread.Comments = slices.DeleteFunc(slices.Clone(thread.Comments), func(comment github.ReviewComment) bool { return comment.Pending })
			if len(thread.Comments) > 0 {
				kept = append(kept, thread)
			}
		}
		client.threads[key] = kept
		client.deletedReviews = append(client.deletedReviews, reviewIdentifier)
		return nil
	}
	return fmt.Errorf("delete review %s: %w", reviewIdentifier, github.ErrNotFound)
}

// DeletedReviews returns the IDs of the successfully deleted reviews.
func (client *Client) DeletedReviews() []string {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	return slices.Clone(client.deletedReviews)
}

// GetReviewComment implements github.Client. It finds the comment in any
// pull request's threads; an unknown ID yields ErrNotFound.
func (client *Client) GetReviewComment(ctx context.Context, commentIdentifier string) (github.ReviewComment, error) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if err := client.begin(ctx, Call{Method: MethodGetReviewComment}); err != nil {
		return github.ReviewComment{}, err
	}
	key, threadIndex, commentIndex, ok := client.findComment(commentIdentifier)
	if !ok {
		return github.ReviewComment{}, fmt.Errorf("load review comment %s: %w", commentIdentifier, github.ErrNotFound)
	}
	return cloneReviewComment(client.threads[key][threadIndex].Comments[commentIndex]), nil
}

// permissions sets what the viewer may do with each comment, as GitHub
// reports it. The viewer's own comments are theirs, with the can-flags as the
// test set them; other people's are not theirs but changeable, as for a
// repository admin (so code must check authorship itself).
func (client *Client) permissions(threads []github.ReviewThread) []github.ReviewThread {
	for i := range threads {
		for j := range threads[i].Comments {
			comment := &threads[i].Comments[j]
			if comment.Author == client.Viewer {
				comment.ViewerDidAuthor = true
				continue
			}
			comment.ViewerDidAuthor, comment.ViewerCanUpdate, comment.ViewerCanDelete = false, true, true
		}
	}
	return threads
}
