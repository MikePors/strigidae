package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// loadCounts delivers message to the harness's view, runs the commands it
// returns and counts the pull request and thread loads they made.
func loadCounts(harness *harness, message any) (pullRequests, threads int) {
	harness.test.Helper()
	before := len(harness.client.Calls())
	_, command := harness.view.Update(message)
	messages(command)
	for _, call := range harness.client.Calls()[before:] {
		switch call.Method {
		case fake.MethodGetPullRequest:
			pullRequests++
		case fake.MethodListReviewThreads:
			threads++
		}
	}
	return pullRequests, threads
}

func TestWriteTimeoutReloadsWhatTheWriteAffects(test *testing.T) {
	timeout := userinterface.WriteError(context.DeadlineExceeded)
	failure := errors.New("boom")
	tests := []struct {
		name string
		// build makes the result of the write, as the write's own command returns it.
		build func(view *View, origin origin, err error) any
		// pullRequests and threads are the loads a timeout starts.
		pullRequests, threads int
		// reloadsOnFailure is set for the writes that already reload after any
		// failure.
		reloadsOnFailure bool
	}{
		{name: "comment", pullRequests: 1, build: func(_ *View, origin origin, err error) any {
			return postedMessage{origin: origin, target: target{kind: kindTopLevel}, body: "text", err: err}
		}},
		{name: "reply", threads: 1, build: func(_ *View, origin origin, err error) any {
			return postedMessage{origin: origin, target: target{kind: kindReply, threadIdentifier: "T1"}, body: "text", err: err}
		}},
		{name: "inline comment", pullRequests: 1, threads: 1, build: func(_ *View, origin origin, err error) any {
			return postedMessage{origin: origin, target: target{kind: kindInline}, body: "text", err: err}
		}},
		{name: "edit", pullRequests: 1, threads: 1, build: func(_ *View, origin origin, err error) any {
			return postedMessage{origin: origin, target: target{kind: kindEdit, threadIdentifier: "T1"}, body: "text", err: err}
		}},
		{name: "delete", pullRequests: 1, threads: 1, build: func(_ *View, origin origin, err error) any {
			return commentDeletedMessage{origin: origin, err: err}
		}},
		{name: "resolve", threads: 1, build: func(_ *View, origin origin, err error) any {
			return threadResolvedMessage{origin: origin, threadIdentifier: "T1", resolve: true, err: err}
		}},
		{name: "react", threads: 1, build: func(_ *View, origin origin, err error) any {
			return reactionToggledMessage{origin: origin, commentIdentifier: "C1", content: github.ReactionContents[0], err: err}
		}},
		{name: "discard review", pullRequests: 1, threads: 1, build: func(_ *View, origin origin, err error) any {
			return reviewDiscardedMessage{origin: origin, err: err}
		}},
		{name: "edit pull request", pullRequests: 1, build: func(_ *View, origin origin, err error) any {
			return pullRequestUpdatedMessage{origin: origin, title: "title", body: "body", err: err}
		}},
		{name: "merge", pullRequests: 1, threads: 1, reloadsOnFailure: true, build: func(view *View, origin origin, err error) any {
			view.merge.merging = true
			return mergedMessage{flow: &view.merge, repository: origin.repository, number: origin.number, err: err}
		}},
		{name: "submit review", pullRequests: 1, threads: 1, reloadsOnFailure: true, build: func(view *View, origin origin, err error) any {
			view.review.submitting = true
			return reviewSubmittedMessage{flow: &view.review, repository: origin.repository, number: origin.number, err: err}
		}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := loaded(test, 100, 24)
			pullRequests, threads := loadCounts(harness, testCase.build(harness.view, harness.view.origin(), timeout))
			if pullRequests != testCase.pullRequests || threads != testCase.threads {
				test.Errorf("after a timeout: %d pull request and %d thread loads, want %d and %d", pullRequests, threads, testCase.pullRequests, testCase.threads)
			}

			pullRequests, threads = loadCounts(harness, testCase.build(harness.view, harness.view.origin(), failure))
			if reloaded := pullRequests+threads > 0; reloaded != testCase.reloadsOnFailure {
				test.Errorf("after another failure: %d pull request and %d thread loads, reloads = %v, want %v", pullRequests, threads, reloaded, testCase.reloadsOnFailure)
			}

			movedOn := harness.view.origin()
			movedOn.number++
			if pullRequests, threads = loadCounts(harness, testCase.build(harness.view, movedOn, timeout)); pullRequests+threads != 0 {
				test.Errorf("a result for another pull request started %d pull request and %d thread loads", pullRequests, threads)
			}
		})
	}
}

// appliedThenBlockedClient applies SetFileViewed on the fake and then hangs
// until the deadline, as a write whose answer never arrives.
type appliedThenBlockedClient struct{ blockingClient }

func (client appliedThenBlockedClient) SetFileViewed(ctx context.Context, pullRequestIdentifier, path string, viewed bool) error {
	if err := client.Client.SetFileViewed(context.WithoutCancel(ctx), pullRequestIdentifier, path, viewed); err != nil {
		return err
	}
	return client.wait(ctx)
}

func TestViewedMarkAfterATimeoutShowsWhatGitHubHas(test *testing.T) {
	tests := []struct {
		name   string
		client func(base blockingClient) github.Client
		viewed bool
	}{
		{"GitHub applied the mark", func(base blockingClient) github.Client { return appliedThenBlockedClient{base} }, true},
		{"GitHub did not", func(base blockingClient) github.Client { return base }, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := loaded(test, 100, 24)
			harness.view.client = testCase.client(blockingClient{Client: harness.client})
			harness.view.timeouts = shortTimeouts
			harness.press("j", "v")
			if !userinterface.IsWriteTimeout(harness.lastErr()) {
				test.Fatalf("error = %v, want a write timeout", harness.lastErr())
			}
			var state github.ViewedState
			for _, file := range harness.view.files() {
				if file.Path == firstFile {
					state = file.Viewed
				}
			}
			if (state == github.FileViewed) != testCase.viewed {
				test.Errorf("%s is %v after the reload, want viewed = %v", firstFile, state, testCase.viewed)
			}
		})
	}
}
