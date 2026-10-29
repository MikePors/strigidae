package diffview

import (
	"slices"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
)

// TestContextMatchesWhatEachRowShows walks every row of the open
// thread and compares the comment Context reports with the one the
// rendered row belongs to, read from the row's own text: the diff line, the
// header and the blank row between comments are on no comment; alice's
// header and body rows are on comment 0 and bob's on comment 1.
func TestContextMatchesWhatEachRowShows(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	if model.Context().OnComment {
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
		context := model.Context()
		index, ok := slices.IndexFunc(context.CommentThread.Comments, func(candidate github.ReviewComment) bool {
			return candidate.Identifier == context.Comment.Identifier && candidate.Author == context.Comment.Author
		}), context.OnComment
		if want < 0 {
			if ok {
				test.Errorf("row %q reports comment %d, want none", text, index)
			}
			continue
		}
		if !ok || len(context.Threads) != 1 || context.CommentThread.Identifier != "open" || index != want {
			test.Errorf("row %q: comment (%d, %v), want %d", text, index, ok, want)
		}
	}
	if len(seen) < 6 {
		test.Errorf("walked only %d rows of the thread: %q", len(seen), seen)
	}
}

func TestContextNotOnAFoldedThread(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	model.CloseThread()
	if model.Context().OnComment {
		test.Error("a folded thread reports a comment")
	}
}

// TestContextCommentAfterFold: SetThreadExpanded lays the rows out at once, so
// the context describes what is drawn: a thread folded before any reload is
// drawn folded and the cursor is on its header, not on a comment.
func TestContextCommentAfterFold(test *testing.T) {
	model := threadModel(test, 100, 80)
	model.goToRow(test, `"log"`)
	model.CursorDown(2)
	if !model.Context().OnComment {
		test.Fatal("setup: the cursor is not on a comment row")
	}
	model.SetThreadExpanded("open", false)
	if model.Context().OnComment {
		test.Error("a thread folded since the last SetThreads reports a comment")
	}
	if text := cursorRowText(model); !strings.Contains(text, "comments") {
		test.Errorf("cursor row = %q, want the folded thread's header", text)
	}
	model.SetThreadExpanded("open", true)
	if !strings.Contains(screen(model), "Yes, for") {
		test.Error("reopened thread is not drawn open")
	}
}
