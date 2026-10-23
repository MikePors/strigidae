package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	githubapi "github.com/cli/go-gh/v2/pkg/api"

	"github.com/MikePors/strigidae/internal/github"
)

// pullRequestQualifiers are the search tokens that already restrict results to pull
// requests. Whole tokens are compared: "is:private" and "-is:pr" do not count.
var pullRequestQualifiers = []string{"is:pr", "type:pr", "is:pull-request", "type:pull-request"}

func hasPullRequestQualifier(query string) bool {
	for _, qualifier := range strings.Fields(query) {
		if slices.Contains(pullRequestQualifiers, qualifier) {
			return true
		}
	}
	return false
}

func afterCursor(pageRequest github.PageRequest) any {
	if pageRequest.After == "" {
		return nil
	}
	return pageRequest.After
}

// ListPullRequests implements github.Client.
func (client *Client) ListPullRequests(ctx context.Context, repository github.Repository, pageRequest github.PageRequest) (github.Page[github.PullRequest], error) {
	var response struct {
		Repository *struct {
			PullRequests graphqlConnection[graphqlPullRequest]
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "first": pageRequest.Size(), "after": afterCursor(pageRequest)}
	if err := client.query(ctx, listPullRequestsQuery, variables, &response); err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repository, err)
	}
	if response.Repository == nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repository, github.ErrNotFound)
	}
	connection := response.Repository.PullRequests
	items, err := mapPullRequests(connection.Nodes)
	if err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("list pull requests in %s: %w", repository, err)
	}
	return github.Page[github.PullRequest]{Items: items, Next: connection.PageInfo.next(), Total: connection.TotalCount}, nil
}

// SearchPullRequests implements github.Client.
func (client *Client) SearchPullRequests(ctx context.Context, query string, pageRequest github.PageRequest) (github.Page[github.PullRequest], error) {
	var response struct {
		Search struct {
			IssueCount int
			PageInfo   graphqlPageInfo
			Nodes      []graphqlPullRequest
		}
	}
	searchQuery := query
	if !hasPullRequestQualifier(searchQuery) {
		searchQuery = "is:pr " + searchQuery
	}
	variables := map[string]any{"query": searchQuery, "first": pageRequest.Size(), "after": afterCursor(pageRequest)}
	if err := client.query(ctx, searchPullRequestsQuery, variables, &response); err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("search pull requests: %w", err)
	}
	items, err := mapPullRequests(response.Search.Nodes)
	if err != nil {
		return github.Page[github.PullRequest]{}, fmt.Errorf("search pull requests: %w", err)
	}
	return github.Page[github.PullRequest]{Items: items, Next: response.Search.PageInfo.next(), Total: response.Search.IssueCount}, nil
}

// GetPullRequest implements github.Client. Files, comments and reviews beyond
// the first 100 are fetched with follow-up requests.
func (client *Client) GetPullRequest(ctx context.Context, repository github.Repository, number int) (github.PullRequestDetail, error) {
	detail, err := client.getPullRequest(ctx, repository, number)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("load pull request %s#%d: %w", repository, number, err)
	}
	return detail, nil
}

func (client *Client) getPullRequest(ctx context.Context, repository github.Repository, number int) (github.PullRequestDetail, error) {
	var response struct {
		Repository *struct {
			PullRequest *graphqlPullRequestDetail
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "number": number}
	err := client.query(ctx, detailQuery(true), variables, &response)
	if isScopeError(err) {
		// Team details need read:org. Retry once without them rather than
		// failing a pull request the user can otherwise read.
		response.Repository = nil
		err = client.query(ctx, detailQuery(false), variables, &response)
	}
	if err != nil {
		return github.PullRequestDetail{}, err
	}
	if response.Repository == nil || response.Repository.PullRequest == nil {
		return github.PullRequestDetail{}, github.ErrNotFound
	}
	node := response.Repository.PullRequest

	files, err := collectMore(ctx, client, repository, number, moreFilesQuery, node.Files)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("files: %w", err)
	}
	comments, err := collectMore(ctx, client, repository, number, moreCommentsQuery, node.Comments)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("comments: %w", err)
	}
	reviews, err := collectMore(ctx, client, repository, number, moreReviewsQuery, node.Reviews)
	if err != nil {
		return github.PullRequestDetail{}, fmt.Errorf("reviews: %w", err)
	}
	return mapDetail(*node, files, comments, reviews)
}

// collectMore returns all nodes of a connection, following its cursor.
func collectMore[Node any](ctx context.Context, client *Client, repository github.Repository, number int, query string, first graphqlConnection[Node]) ([]Node, error) {
	nodes := first.Nodes
	cursor := first.PageInfo.next()
	seen := map[string]bool{}
	for cursor != "" {
		if seen[cursor] {
			return nil, fmt.Errorf("pagination did not advance (repeated cursor %q)", cursor)
		}
		seen[cursor] = true
		var response struct {
			Repository *struct {
				PullRequest *struct{ Conn graphqlConnection[Node] }
			}
		}
		variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "number": number, "after": cursor}
		if err := client.query(ctx, query, variables, &response); err != nil {
			return nil, err
		}
		if response.Repository == nil || response.Repository.PullRequest == nil {
			return nil, github.ErrNotFound
		}
		connection := response.Repository.PullRequest.Conn
		nodes = append(nodes, connection.Nodes...)
		cursor = connection.PageInfo.next()
	}
	return nodes, nil
}

// GetDiff implements github.Client.
func (client *Client) GetDiff(ctx context.Context, repository github.Repository, number int) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", url.PathEscape(repository.Owner), url.PathEscape(repository.Name), number)
	response, err := client.diff.RequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		var httpErr *githubapi.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotAcceptable {
			return "", fmt.Errorf("load diff of %s#%d: %w: %w", repository, number, github.ErrDiffTooLarge, err)
		}
		return "", fmt.Errorf("load diff of %s#%d: %w", repository, number, client.mapError(err))
	}
	defer func() { _ = response.Body.Close() }()
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("read diff of %s#%d: %w", repository, number, err)
	}
	return string(bytes), nil
}

// ListReviewThreads implements github.Client.
func (client *Client) ListReviewThreads(ctx context.Context, repository github.Repository, number int, pageRequest github.PageRequest) (github.Page[github.ReviewThread], error) {
	var response struct {
		Repository *struct {
			PullRequest *struct {
				ReviewThreads graphqlConnection[graphqlThread]
			}
		}
	}
	variables := map[string]any{"owner": repository.Owner, "name": repository.Name, "number": number, "first": pageRequest.Size(), "after": afterCursor(pageRequest)}
	if err := client.query(ctx, reviewThreadsQuery, variables, &response); err != nil {
		return github.Page[github.ReviewThread]{}, fmt.Errorf("list review threads of %s#%d: %w", repository, number, err)
	}
	if response.Repository == nil || response.Repository.PullRequest == nil {
		return github.Page[github.ReviewThread]{}, fmt.Errorf("list review threads of %s#%d: %w", repository, number, github.ErrNotFound)
	}
	connection := response.Repository.PullRequest.ReviewThreads
	items := make([]github.ReviewThread, 0, len(connection.Nodes))
	for _, node := range connection.Nodes {
		items = append(items, mapThread(node))
	}
	return github.Page[github.ReviewThread]{Items: items, Next: connection.PageInfo.next(), Total: connection.TotalCount}, nil
}

// ReplyToThread implements github.Client.
func (client *Client) ReplyToThread(ctx context.Context, threadIdentifier, body string) (github.ReviewComment, error) {
	var response struct {
		AddPullRequestReviewThreadReply *struct{ Comment *graphqlReviewComment }
	}
	variables := map[string]any{"threadId": threadIdentifier, "body": body}
	if err := client.query(ctx, replyToThreadMutation, variables, &response); err != nil {
		return github.ReviewComment{}, fmt.Errorf("reply to thread: %w", err)
	}
	if response.AddPullRequestReviewThreadReply == nil || response.AddPullRequestReviewThreadReply.Comment == nil {
		return github.ReviewComment{}, fmt.Errorf("reply to thread: %w", github.ErrNotFound)
	}
	return mapReviewComment(*response.AddPullRequestReviewThreadReply.Comment), nil
}

// AddComment implements github.Client. It uses REST because that call needs
// only the repository and number, not the pull request's node ID.
func (client *Client) AddComment(ctx context.Context, repository github.Repository, number int, body string) (github.IssueComment, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return github.IssueComment{}, fmt.Errorf("add comment: %w", err)
	}
	var response struct {
		NodeID    string `json:"node_id"`
		HTMLURL   string `json:"html_url"`
		Body      string
		CreatedAt time.Time `json:"created_at"`
		User      *graphqlActor
	}
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments", url.PathEscape(repository.Owner), url.PathEscape(repository.Name), number)
	if err := client.rest.DoWithContext(ctx, "POST", path, bytes.NewReader(payload), &response); err != nil {
		return github.IssueComment{}, fmt.Errorf("add comment to %s#%d: %w", repository, number, client.mapError(err))
	}
	return github.IssueComment{Identifier: response.NodeID, Author: response.User.login(), Body: response.Body, CreatedAt: response.CreatedAt, URL: response.HTMLURL}, nil
}

// SetFileViewed implements github.Client.
func (client *Client) SetFileViewed(ctx context.Context, pullRequestIdentifier, path string, viewed bool) error {
	query := unmarkFileViewedMutation
	if viewed {
		query = markFileViewedMutation
	}
	var response json.RawMessage
	// The caller names the action and the file, so the error is not wrapped again.
	return client.query(ctx, query, map[string]any{"pullRequestId": pullRequestIdentifier, "path": path}, &response)
}
