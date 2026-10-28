package diffview

import (
	"strings"
	"testing"
)

// TestCommentAtCursorMatchesWhatEachRowShows walks every row of the open
// thread and compares the comment CommentAtCursor reports with the one the
// rendered row belongs to, read from the row's own text: the diff line, the
// header and the blank row between comments are on no comment; alice's
// header and body rows are on comment 0 and bob's on comment 1.
func TestCommentAtCursorMatchesWhatEachRowShows(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	if _, _, ok := model.CommentAtCursor(); ok {
		test.Fatal("the diff line reports a comment")
	}
	classify := func(text string) (want int, known bool) {
		switch {
		case strings.Contains(text, "comments"):
			return -1, true // the thread header
		case strings.Contains(text, "alice") || strings.Contains(text, "Do we need"):
			return 0, true
		case strings.Contains(text, "bob") || strings.Contains(text, "Yes, for"):
			return 1, true
		case strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), ">")) == "│":
			return -1, true // the blank row between two comments
		}
		return 0, false
	}
	var seen []string
	for range 12 {
		model.CursorDown(1)
		text := cursorRowText(model)
		want, known := classify(text)
		if !known {
			break // left the thread
		}
		seen = append(seen, strings.TrimSpace(text))
		reviewThread, index, ok := model.CommentAtCursor()
		if want < 0 {
			if ok {
				test.Errorf("row %q reports comment %d, want none", text, index)
			}
			continue
		}
		if !ok || reviewThread.Identifier != "open" || index != want {
			test.Errorf("row %q: comment (%d, %v), want %d", text, index, ok, want)
		}
	}
	if len(seen) < 6 {
		test.Errorf("walked only %d rows of the thread: %q", len(seen), seen)
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
