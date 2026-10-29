package workspace

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func TestReviewCommentTarget(test *testing.T) {
	thread := github.ReviewThread{Identifier: "t-1", Path: "a.go", Comments: []github.ReviewComment{{Identifier: "c-1"}, {Identifier: "c-2"}}}
	reactions := []github.ReactionSummary{{Content: github.ReactionHeart, Count: 2, ViewerReacted: true}}
	tests := []struct {
		name    string
		comment github.ReviewComment
		want    commentTarget
	}{
		{
			"someone else's published comment",
			github.ReviewComment{Identifier: "c-2", Author: "bob", Reactions: reactions, ViewerCanUpdate: true, ViewerCanDelete: true},
			commentTarget{kind: commentReview, identifier: "c-2", author: "bob", reactions: reactions, viewerCanUpdate: true, viewerCanDelete: true, thread: &thread},
		},
		{
			"own pending comment",
			github.ReviewComment{Identifier: "c-1", Author: "me", Pending: true, ViewerDidAuthor: true, ViewerCanUpdate: true, ViewerCanDelete: true},
			commentTarget{kind: commentReview, identifier: "c-1", author: "me", pending: true, viewerDidAuthor: true, viewerCanUpdate: true, viewerCanDelete: true, thread: &thread},
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got := reviewCommentTarget(testCase.comment, thread)
			if difference := cmp.Diff(testCase.want, got, cmp.AllowUnexported(commentTarget{})); difference != "" {
				test.Errorf("target (-want +got):\n%s", difference)
			}
		})
	}
}

// TestResolveTargetMapsTheDiffContext: the comment-level target is the comment
// under the cursor, with the whole thread it is in.
func TestResolveTargetMapsTheDiffContext(test *testing.T) {
	inliner := newInliner(test)
	inliner.seedTargets()
	tests := []struct {
		name       string
		row        string
		identifier string
		own        bool
	}{
		{"own first comment", "first mine", firstOwnIdentifier, true},
		{"own second comment", "second mine", secondOwnIdentifier, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			inliner.toLineContaining(testCase.row)
			resolved, command := inliner.view.resolveTarget(request{level: levelComment, words: "react to it"})
			if command != nil {
				test.Fatal("no target on a comment")
			}
			target := resolved.comment
			if target.kind != commentReview || target.identifier != testCase.identifier || target.viewerDidAuthor != testCase.own {
				test.Errorf("target = %+v", target)
			}
			if target.thread == nil || len(target.thread.Comments) != 2 {
				test.Errorf("thread = %+v, want the two-comment thread", target.thread)
			}
		})
	}
	test.Run("someone else's comment", func(test *testing.T) {
		inliner.toComment(0)
		resolved, command := inliner.view.resolveTarget(request{level: levelComment, words: "react to it"})
		if command != nil {
			test.Fatal("no target on a comment")
		}
		if resolved.comment.viewerDidAuthor || resolved.comment.thread == nil || resolved.comment.thread.Identifier != "t-open" {
			test.Errorf("target = %+v", resolved.comment)
		}
	})
}
