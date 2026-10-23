// Package fake provides an in-memory github.Client for tests and demos. Seed
// it with the Set* methods, call it like the real client, then inspect what
// was recorded with Calls, Replies and Comments.
//
// The fake mirrors the real client where UI code can observe it: it returns
// only open pull requests newest-updated first, rejects empty comment bodies,
// answers ErrNotFound for unknown pull requests and threads, and updates the
// comment counts that later reads report. Like the real client it never shares
// memory with callers: everything stored and returned is deep-copied, so a
// view can append to or edit what it received without affecting the fake.
//
// Seeded data is returned as given, including pending (draft) reviews and
// comments; nothing is filtered by viewer.
package fake

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// Method names recorded in Call.Method and accepted by SetError.
const (
	MethodListPullRequests   = "ListPullRequests"
	MethodSearchPullRequests = "SearchPullRequests"
	MethodGetPullRequest     = "GetPullRequest"
	MethodGetDiff            = "GetDiff"
	MethodListReviewThreads  = "ListReviewThreads"
	MethodReplyToThread      = "ReplyToThread"
	MethodAddComment         = "AddComment"
	MethodGetReviewStatus    = "GetReviewStatus"
	MethodSubmitReview       = "SubmitReview"
	MethodSetFileViewed      = "SetFileViewed"
	MethodGetMergeStatus     = "GetMergeStatus"
	MethodMergePullRequest   = "MergePullRequest"
)

// Call is one recorded call with the arguments that apply to its method.
type Call struct {
	Method           string
	Repository       github.Repository
	Number           int
	Query            string
	Page             github.PageRequest
	ThreadIdentifier string
	Body             string
	// PullRequestID, Path and Viewed are set by the methods that take them:
	// PullRequestID by SetFileViewed, StartReview and MergePullRequest, Path by
	// SetFileViewed and AddReviewThread, Viewed by SetFileViewed.
	PullRequestIdentifier string
	Path                  string
	Viewed                bool
}

// Reply is a recorded successful ReplyToThread call.
type Reply struct {
	ThreadIdentifier string
	Body             string
}

// Comment is a recorded successful AddComment call.
type Comment struct {
	Repository github.Repository
	Number     int
	Body       string
}

type pullRequestKey struct {
	repository github.Repository
	number     int
}

// Client implements github.Client in memory. The zero value is not usable;
// call New. Cursors are decimal offsets. All methods are safe for concurrent use.
type Client struct {
	// Viewer is the login used as the author of replies and comments.
	Viewer string
	// Now supplies timestamps for created comments.
	Now func() time.Time

	mu             sync.Mutex
	lists          map[github.Repository][]github.PullRequest
	search         map[string][]github.PullRequest
	details        map[pullRequestKey]github.PullRequestDetail
	diffs          map[pullRequestKey]string
	threads        map[pullRequestKey][]github.ReviewThread
	errs           map[string]error
	calls          []Call
	replies        []Reply
	comments       []Comment
	pending        map[pullRequestKey]github.PendingReview
	reviews        []Submission
	nextIdentifier int
	merge          map[github.Repository]MergeSettings
	merges         []Merge
	added          []github.AddThreadInput
	started        []StartedReview
	deleted        []string
}

var _ github.Client = (*Client)(nil)

// New returns an empty fake client whose viewer is "me".
func New() *Client {
	return &Client{
		Viewer:  "me",
		Now:     time.Now,
		lists:   map[github.Repository][]github.PullRequest{},
		search:  map[string][]github.PullRequest{},
		details: map[pullRequestKey]github.PullRequestDetail{},
		diffs:   map[pullRequestKey]string{},
		threads: map[pullRequestKey][]github.ReviewThread{},
		errs:    map[string]error{},
		pending: map[pullRequestKey]github.PendingReview{},
		merge:   map[github.Repository]MergeSettings{},
	}
}

func cloneReactions(r []github.ReactionSummary) []github.ReactionSummary { return slices.Clone(r) }

func cloneIssueComment(c github.IssueComment) github.IssueComment {
	c.Reactions = cloneReactions(c.Reactions)
	return c
}

func cloneReviewComment(c github.ReviewComment) github.ReviewComment {
	c.Reactions = cloneReactions(c.Reactions)
	return c
}

func cloneThread(t github.ReviewThread) github.ReviewThread {
	cs := make([]github.ReviewComment, len(t.Comments))
	for i, c := range t.Comments {
		cs[i] = cloneReviewComment(c)
	}
	if t.Comments == nil {
		cs = nil
	}
	t.Comments = cs
	return t
}

func cloneThreads(ts []github.ReviewThread) []github.ReviewThread {
	if ts == nil {
		return nil
	}
	out := make([]github.ReviewThread, len(ts))
	for i, t := range ts {
		out[i] = cloneThread(t)
	}
	return out
}

func cloneDetail(d github.PullRequestDetail) github.PullRequestDetail {
	d.Labels = slices.Clone(d.Labels)
	d.ReviewRequests = slices.Clone(d.ReviewRequests)
	d.Files = slices.Clone(d.Files)
	d.Reviews = slices.Clone(d.Reviews)
	if d.Comments != nil {
		cs := make([]github.IssueComment, len(d.Comments))
		for i, c := range d.Comments {
			cs[i] = cloneIssueComment(c)
		}
		d.Comments = cs
	}
	return d
}

// SetPullRequests sets the pull requests known for repo. ListPullRequests
// returns the open ones, most recently updated first.
func (c *Client) SetPullRequests(repo github.Repository, prs []github.PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists[repo] = slices.Clone(prs)
}

// SetSearchResults sets what SearchPullRequests returns for exactly this
// query, in the given order. Unseeded queries return an empty page.
func (c *Client) SetSearchResults(query string, prs []github.PullRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.search[query] = slices.Clone(prs)
}

// SetPullRequest sets the detail returned for d.Repo and d.Number.
func (c *Client) SetPullRequest(d github.PullRequestDetail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.details[pullRequestKey{d.Repository, d.Number}] = cloneDetail(d)
}

// SetDiff sets the raw diff returned for a pull request.
func (c *Client) SetDiff(repo github.Repository, number int, diff string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diffs[pullRequestKey{repo, number}] = diff
}

// SetReviewThreads sets the threads returned for a pull request.
func (c *Client) SetReviewThreads(repo github.Repository, number int, threads []github.ReviewThread) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.threads[pullRequestKey{repo, number}] = c.permissions(cloneThreads(threads))
}

// SetError makes the named method fail with err until cleared with a nil err.
// An empty method applies to every method that has no error of its own.
func (c *Client) SetError(method string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		delete(c.errs, method)
		return
	}
	c.errs[method] = err
}

// Calls returns every call made so far with its arguments, in order,
// including calls that failed.
func (c *Client) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.calls)
}

// Replies returns the recorded successful ReplyToThread calls.
func (c *Client) Replies() []Reply {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.replies)
}

// Comments returns the recorded successful AddComment calls.
func (c *Client) Comments() []Comment {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.comments)
}

// begin records the call and returns the injected error, if any. Callers hold c.mu.
func (c *Client) begin(ctx context.Context, call Call) error {
	c.calls = append(c.calls, call)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err, ok := c.errs[call.Method]; ok {
		return err
	}
	return c.errs[""]
}

func paginate[T any](items []T, p github.PageRequest) (github.Page[T], error) {
	start := 0
	if p.After != "" {
		n, err := strconv.Atoi(p.After)
		if err != nil || n < 0 || n > len(items) {
			return github.Page[T]{}, fmt.Errorf("invalid cursor %q", p.After)
		}
		start = n
	}
	end := min(start+p.Size(), len(items))
	page := github.Page[T]{Items: slices.Clone(items[start:end]), Total: len(items)}
	if end < len(items) {
		page.Next = strconv.Itoa(end)
	}
	return page, nil
}

// ListPullRequests implements github.Client.
func (c *Client) ListPullRequests(ctx context.Context, repo github.Repository, page github.PageRequest) (github.Page[github.PullRequest], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodListPullRequests, Repository: repo, Page: page}); err != nil {
		return github.Page[github.PullRequest]{}, err
	}
	all, ok := c.lists[repo]
	if !ok {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repo, github.ErrNotFound)
	}
	var open []github.PullRequest
	for _, pr := range all {
		if pr.State == "" || pr.State == github.PullRequestStateOpen {
			open = append(open, pr)
		}
	}
	slices.SortStableFunc(open, func(a, b github.PullRequest) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return paginate(open, page)
}

// SearchPullRequests implements github.Client.
func (c *Client) SearchPullRequests(ctx context.Context, query string, page github.PageRequest) (github.Page[github.PullRequest], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodSearchPullRequests, Query: query, Page: page}); err != nil {
		return github.Page[github.PullRequest]{}, err
	}
	return paginate(c.search[query], page)
}

// GetPullRequest implements github.Client.
func (c *Client) GetPullRequest(ctx context.Context, repo github.Repository, number int) (github.PullRequestDetail, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetPullRequest, Repository: repo, Number: number}); err != nil {
		return github.PullRequestDetail{}, err
	}
	d, ok := c.details[pullRequestKey{repo, number}]
	if !ok {
		return github.PullRequestDetail{}, fmt.Errorf("load pull request %s#%d: %w", repo, number, github.ErrNotFound)
	}
	return cloneDetail(d), nil
}

// GetDiff implements github.Client.
func (c *Client) GetDiff(ctx context.Context, repo github.Repository, number int) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodGetDiff, Repository: repo, Number: number}); err != nil {
		return "", err
	}
	d, ok := c.diffs[pullRequestKey{repo, number}]
	if !ok {
		return "", fmt.Errorf("load diff of %s#%d: %w", repo, number, github.ErrNotFound)
	}
	return d, nil
}

// ListReviewThreads implements github.Client. A pull request with no threads
// set yields an empty page.
func (c *Client) ListReviewThreads(ctx context.Context, repo github.Repository, number int, page github.PageRequest) (github.Page[github.ReviewThread], error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodListReviewThreads, Repository: repo, Number: number, Page: page}); err != nil {
		return github.Page[github.ReviewThread]{}, err
	}
	p, err := paginate(c.threads[pullRequestKey{repo, number}], page)
	p.Items = cloneThreads(p.Items)
	return p, err
}

// ReplyToThread implements github.Client. The reply is appended to the thread
// so later ListReviewThreads calls include it.
func (c *Client) ReplyToThread(ctx context.Context, threadID, body string) (github.ReviewComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodReplyToThread, ThreadIdentifier: threadID, Body: body}); err != nil {
		return github.ReviewComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return github.ReviewComment{}, fmt.Errorf("reply to thread %s: empty body", threadID)
	}
	for _, threads := range c.threads {
		for i := range threads {
			if threads[i].Identifier != threadID {
				continue
			}
			c.nextIdentifier++
			rc := github.ReviewComment{
				Identifier: "fake-review-comment-" + strconv.Itoa(c.nextIdentifier),
				Author:     c.Viewer,
				Body:       body,
				CreatedAt:  c.Now(),

				ViewerCanUpdate: true, ViewerCanDelete: true, ViewerDidAuthor: true,
			}
			// Build a fresh slice so slices handed out earlier are never written to.
			threads[i].Comments = append(slices.Clip(threads[i].Comments), rc)
			c.replies = append(c.replies, Reply{ThreadIdentifier: threadID, Body: body})
			return rc, nil
		}
	}
	return github.ReviewComment{}, fmt.Errorf("reply to thread %s: %w", threadID, github.ErrNotFound)
}

// AddComment implements github.Client. It fails with ErrNotFound for a pull
// request the fake has no data for, and otherwise bumps the comment count in
// the detail, the repo list and search results.
func (c *Client) AddComment(ctx context.Context, repo github.Repository, number int, body string) (github.IssueComment, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodAddComment, Repository: repo, Number: number, Body: body}); err != nil {
		return github.IssueComment{}, err
	}
	if strings.TrimSpace(body) == "" {
		return github.IssueComment{}, fmt.Errorf("add comment to %s#%d: empty body", repo, number)
	}
	key := pullRequestKey{repo, number}
	detail, hasDetail := c.details[key]
	known := hasDetail
	bump := func(prs []github.PullRequest) {
		for i := range prs {
			if prs[i].Repository == repo && prs[i].Number == number {
				prs[i].CommentCount++
				known = true
			}
		}
	}
	for _, prs := range c.lists {
		bump(prs)
	}
	for _, prs := range c.search {
		bump(prs)
	}
	if !known {
		return github.IssueComment{}, fmt.Errorf("add comment to %s#%d: %w", repo, number, github.ErrNotFound)
	}
	c.nextIdentifier++
	ic := github.IssueComment{
		Identifier: "fake-comment-" + strconv.Itoa(c.nextIdentifier),
		Author:     c.Viewer,
		Body:       body,
		CreatedAt:  c.Now(),
	}
	if hasDetail {
		detail.Comments = append(slices.Clip(detail.Comments), ic)
		detail.CommentCount++
		c.details[key] = detail
	}
	c.comments = append(c.comments, Comment{Repository: repo, Number: number, Body: body})
	return ic, nil
}

// SetFileViewed implements github.Client. It updates the file's state in the
// pull request detail with the matching ID, so later reads report it; an
// unknown pull request or path fails with ErrNotFound.
func (c *Client) SetFileViewed(ctx context.Context, pullRequestID, path string, viewed bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.begin(ctx, Call{Method: MethodSetFileViewed, PullRequestIdentifier: pullRequestID, Path: path, Viewed: viewed}); err != nil {
		return err
	}
	state := github.FileUnviewed
	if viewed {
		state = github.FileViewed
	}
	for key, d := range c.details {
		if d.Identifier != pullRequestID {
			continue
		}
		for i := range d.Files {
			if d.Files[i].Path == path {
				d.Files = slices.Clone(d.Files)
				d.Files[i].Viewed = state
				c.details[key] = d
				return nil
			}
		}
	}
	return fmt.Errorf("mark %s as viewed in %s: %w", path, pullRequestID, github.ErrNotFound)
}
