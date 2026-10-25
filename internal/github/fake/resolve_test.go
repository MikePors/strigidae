package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestResolveAndUnresolveReviewThread(test *testing.T) {
	ctx := context.Background()
	client := seeded()
	client.Viewer = "me"

	resolved, err := client.ResolveReviewThread(ctx, "t1")
	if err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff(github.ThreadResolution{Identifier: "t1", IsResolved: true, ResolvedBy: "me"}, resolved); diff != "" {
		test.Errorf("resolved (-want +got):\n%s", diff)
	}
	page, err := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if err != nil {
		test.Fatal(err)
	}
	if !page.Items[0].IsResolved || page.Items[0].ResolvedBy != "me" || page.Items[1].IsResolved {
		test.Errorf("threads after resolve = %+v", page.Items)
	}

	unresolved, err := client.UnresolveReviewThread(ctx, "t1")
	if err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff(github.ThreadResolution{Identifier: "t1"}, unresolved); diff != "" {
		test.Errorf("unresolved (-want +got):\n%s", diff)
	}
	page, _ = client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
	if page.Items[0].IsResolved || page.Items[0].ResolvedBy != "" {
		test.Errorf("thread after unresolve = %+v", page.Items[0])
	}

	var methods []string
	for _, call := range client.Calls() {
		if call.ThreadIdentifier == "t1" {
			methods = append(methods, call.Method)
		}
	}
	if diff := cmp.Diff([]string{fake.MethodResolveReviewThread, fake.MethodUnresolveReviewThread}, methods); diff != "" {
		test.Errorf("calls (-want +got):\n%s", diff)
	}
}

func TestResolveReviewThreadErrors(test *testing.T) {
	ctx := context.Background()
	client := seeded()
	if _, err := client.ResolveReviewThread(ctx, "missing"); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("unknown thread: err = %v, want ErrNotFound", err)
	}
	injected := errors.New("boom")
	client.SetError(fake.MethodUnresolveReviewThread, injected)
	if _, err := client.UnresolveReviewThread(ctx, "t1"); !errors.Is(err, injected) {
		test.Errorf("injected: err = %v", err)
	}
}
