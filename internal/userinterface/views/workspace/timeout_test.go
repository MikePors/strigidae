package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// blockingClient makes the calls under test wait until their context ends, as
// a connection that hangs does.
type blockingClient struct{ *fake.Client }

func (blockingClient) GetPullRequest(ctx context.Context, _ github.Repository, _ int) (github.PullRequestDetail, error) {
	<-ctx.Done()
	return github.PullRequestDetail{}, ctx.Err()
}

func (blockingClient) GetDiff(ctx context.Context, _ github.Repository, _ int) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (blockingClient) ListReviewThreads(ctx context.Context, _ github.Repository, _ int, _ github.PageRequest) (github.Page[github.ReviewThread], error) {
	<-ctx.Done()
	return github.Page[github.ReviewThread]{}, ctx.Err()
}

func (blockingClient) AddComment(ctx context.Context, _ github.Repository, _ int, _ string) (github.IssueComment, error) {
	<-ctx.Done()
	return github.IssueComment{}, ctx.Err()
}

func (blockingClient) ReplyToThread(ctx context.Context, _, _ string) (github.ReviewComment, error) {
	<-ctx.Done()
	return github.ReviewComment{}, ctx.Err()
}

func (blockingClient) SetFileViewed(ctx context.Context, _, _ string, _ bool) error {
	<-ctx.Done()
	return ctx.Err()
}

var shortTimeouts = userinterface.Timeouts{Request: 5 * time.Millisecond, Load: 5 * time.Millisecond}

func TestLoadTimesOut(test *testing.T) {
	harness := newHarness(test, 100, 24)
	harness.view.client = blockingClient{harness.client}
	harness.view.timeouts = shortTimeouts
	harness.run(harness.view.Init())
	if harness.view.Loading() {
		test.Error("still loading after the deadline passed")
	}
	if err := harness.lastErr(); !errors.Is(err, context.DeadlineExceeded) {
		test.Errorf("error = %v, want a deadline error", err)
	}
}

func TestWritesTimeOutAndSayTheyMayHaveGoneThrough(test *testing.T) {
	tests := []struct {
		name  string
		start func(harness *harness)
	}{
		{"comment", func(harness *harness) {
			harness.view.writeStarted()
			harness.run(harness.view.post(target{}, "body"))
		}},
		{"reply", func(harness *harness) {
			harness.view.writeStarted()
			harness.run(harness.view.post(target{kind: kindReply, threadIdentifier: "T1"}, "body"))
		}},
		{"viewed mark", func(harness *harness) {
			harness.press("j", "v")
		}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := loaded(test, 100, 24)
			harness.view.client = blockingClient{harness.client}
			harness.view.timeouts = shortTimeouts
			testCase.start(harness)
			err := harness.lastErr()
			if err == nil || !strings.Contains(err.Error(), "it may have gone through") {
				test.Errorf("error = %v, want it to say the write may have gone through", err)
			}
			if harness.view.writes != 0 {
				test.Errorf("writes in flight = %d, want 0", harness.view.writes)
			}
		})
	}
}
