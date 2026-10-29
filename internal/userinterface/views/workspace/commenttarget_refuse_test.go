package workspace

import (
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// Kinds the Overview will produce have no thread; e and D refuse them instead
// of reading the thread.
func TestEditAndDeleteRefuseTargetsThatAreNotReviewComments(test *testing.T) {
	for _, kind := range []commentKind{commentTopLevel, commentDescription} {
		refusal := reviewCommentOnly(commentTarget{kind: kind, viewerDidAuthor: true}, "edit it")
		if refusal == nil {
			test.Fatalf("kind %d: not refused", kind)
		}
		if _, ok := refusal().(userinterface.InfoMessage); !ok {
			test.Errorf("kind %d: refusal is not a notice", kind)
		}
	}
	if reviewCommentOnly(reviewCommentTarget(github.ReviewComment{}, github.ReviewThread{}), "edit it") != nil {
		test.Error("a review comment was refused")
	}
}

// A fold applied before the thread data is reloaded is what is drawn, so e
// finds no comment under the cursor.
func TestFoldBeforeReloadIsWhatActionsTarget(test *testing.T) {
	inliner := newInliner(test)
	inliner.seedTargets()
	inliner.toLineContaining("first mine")
	inliner.view.filePane.SetThreadExpanded(mineIdentifier, false)
	inliner.press("e")
	if got := inliner.lastInfo(); got != noticeEdit {
		test.Errorf("notice = %q, want %q", got, noticeEdit)
	}
	if len(inliner.openedOn) != 0 {
		test.Errorf("editor opened on %q", inliner.openedOn)
	}
}
