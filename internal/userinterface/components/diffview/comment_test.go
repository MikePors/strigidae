package diffview

import "testing"

// TestCommentAtCursorFollowsEachRowOfTheThread walks every row of the open
// thread: the diff line and the header are not on a comment, the rows of the
// first comment report index 0, and those of the second report index 1.
func TestCommentAtCursorFollowsEachRowOfTheThread(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	if _, _, ok := model.CommentAtCursor(); ok {
		test.Fatal("the diff line reports a comment")
	}
	model.CursorDown(1) // the header
	if _, _, ok := model.CommentAtCursor(); ok {
		test.Fatal("the thread header reports a comment")
	}
	var seen []int
	for range 40 {
		model.CursorDown(1)
		reviewThread, index, ok := model.CommentAtCursor()
		if !ok {
			break
		}
		if reviewThread.Identifier != "open" {
			test.Fatalf("thread = %q", reviewThread.Identifier)
		}
		if len(seen) == 0 || seen[len(seen)-1] != index {
			seen = append(seen, index)
		}
	}
	if len(seen) != 2 || seen[0] != 0 || seen[1] != 1 {
		test.Errorf("comment indexes in row order = %v, want [0 1]", seen)
	}
}

func TestCommentAtCursorNotOnAFoldedThread(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	model.CloseThread()
	if _, _, ok := model.CommentAtCursor(); ok {
		test.Error("a folded thread reports a comment")
	}
}
