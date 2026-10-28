package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestAddAndRemoveReaction(test *testing.T) {
	ctx := context.Background()
	client := fake.New()
	client.SetReviewThreads(repository, 1, []github.ReviewThread{{Identifier: "t1", Comments: []github.ReviewComment{
		{Identifier: "c1", Reactions: []github.ReactionSummary{{Content: github.ReactionHeart, Count: 2}}},
		{Identifier: "c2"},
	}}})

	steps := []struct {
		name    string
		remove  bool
		content github.ReactionContent
		want    []github.ReactionSummary
	}{
		{"add a new kind, in GitHub's order", false, github.ReactionThumbsUp, []github.ReactionSummary{
			{Content: github.ReactionThumbsUp, Count: 1, ViewerReacted: true}, {Content: github.ReactionHeart, Count: 2},
		}},
		{"add to an existing kind", false, github.ReactionHeart, []github.ReactionSummary{
			{Content: github.ReactionThumbsUp, Count: 1, ViewerReacted: true}, {Content: github.ReactionHeart, Count: 3, ViewerReacted: true},
		}},
		{"adding again changes nothing", false, github.ReactionHeart, []github.ReactionSummary{
			{Content: github.ReactionThumbsUp, Count: 1, ViewerReacted: true}, {Content: github.ReactionHeart, Count: 3, ViewerReacted: true},
		}},
		{"remove drops a group that reaches zero", true, github.ReactionThumbsUp, []github.ReactionSummary{
			{Content: github.ReactionHeart, Count: 3, ViewerReacted: true},
		}},
		{"removing one the viewer lacks changes nothing", true, github.ReactionEyes, []github.ReactionSummary{
			{Content: github.ReactionHeart, Count: 3, ViewerReacted: true},
		}},
	}
	for _, step := range steps {
		var err error
		if step.remove {
			err = client.RemoveReaction(ctx, "c1", step.content)
		} else {
			err = client.AddReaction(ctx, "c1", step.content)
		}
		if err != nil {
			test.Fatalf("%s: %v", step.name, err)
		}
		page, _ := client.ListReviewThreads(ctx, repository, 1, github.PageRequest{})
		if diff := cmp.Diff(step.want, page.Items[0].Comments[0].Reactions); diff != "" {
			test.Errorf("%s (-want +got):\n%s", step.name, diff)
		}
		if got := page.Items[0].Comments[1].Reactions; len(got) != 0 {
			test.Errorf("%s: other comment changed: %v", step.name, got)
		}
	}
	calls := client.Calls()
	last := calls[len(calls)-2] // the call before the final list
	if last.Method != fake.MethodRemoveReaction || last.SubjectIdentifier != "c1" || last.Reaction != github.ReactionEyes {
		test.Errorf("last call = %+v", last)
	}
}

func TestReactionErrors(test *testing.T) {
	ctx := context.Background()
	client := fake.New()
	if err := client.AddReaction(ctx, "missing", github.ReactionEyes); !errors.Is(err, github.ErrNotFound) {
		test.Errorf("unknown comment: err = %v, want not found", err)
	}
	injected := errors.New("boom")
	client.SetError(fake.MethodRemoveReaction, injected)
	if err := client.RemoveReaction(ctx, "missing", github.ReactionEyes); !errors.Is(err, injected) {
		test.Errorf("injected: err = %v", err)
	}
}
