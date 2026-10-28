package diffview

import (
	"slices"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
)

// Context is everything at the cursor, from the file down to a comment. Each
// level is set only when the cursor is on it.
type Context struct {
	// File is the file the pane shows; nil with no file or when a message
	// replaces the diff.
	File *diff.File
	// InHunk reports whether the cursor is in a hunk: on its header, on one of
	// its lines, or on a thread below one of them. It is false with no file,
	// on a notice, and on a thread that is on no line.
	InHunk bool
	// Hunk is the index into File.Hunks of the hunk the cursor is in; only
	// meaningful when InHunk.
	Hunk int
	// HasLines reports whether Lines is set.
	HasLines bool
	// Lines is the range a new comment would go on: the visual selection while
	// one is in progress, else the cursor line. It is not set when the cursor
	// is not on a diff line, or when the selection is invalid.
	Lines Range
	// Threads are the shown threads the cursor is on: the one whose rows it is
	// in, or those anchored to the diff line it is on.
	Threads []github.ReviewThread
	// OnComment reports whether the cursor is on the rows of a comment of an
	// expanded thread; it is false on a header, on the blank rows between
	// comments, and in a folded thread.
	OnComment bool
	// CommentIndex is the index in Threads[0].Comments of the comment the
	// cursor is on; only meaningful when OnComment.
	CommentIndex int
}

// Context returns what is at the cursor.
func (model *Model) Context() Context {
	context := Context{File: model.file}
	context.Hunk, context.InHunk = model.cursorHunk()
	context.Lines, context.HasLines = model.anchorRange()
	for _, threadIndex := range model.threadsAtCursor() {
		context.Threads = append(context.Threads, model.fileThreads[threadIndex])
	}
	context.CommentIndex, context.OnComment = model.cursorComment()
	return context
}

// cursorHunk is the index of the hunk the cursor is in.
func (model *Model) cursorHunk() (hunk int, ok bool) {
	if model.file == nil || len(model.rows) == 0 { // SetMessage clears the file
		return 0, false
	}
	cursorRow := model.rows[model.window.Cursor()]
	if cursorRow.threadRow != nil {
		orphan := slices.Contains(model.orphans, cursorRow.threadRow.threadIndex)
		if orphan || cursorRow.baseIndex < 0 || cursorRow.baseIndex >= len(model.base) {
			return 0, false
		}
		cursorRow = model.base[cursorRow.baseIndex]
	}
	if cursorRow.message {
		return 0, false
	}
	return cursorRow.hunk, true
}

// anchorRange is the range a comment would be written on: the selection while
// one is in progress, else the cursor line.
func (model *Model) anchorRange() (lineRange Range, ok bool) {
	if model.visual {
		return model.selection()
	}
	cursorLine, ok := model.Cursor()
	if !ok {
		return Range{}, false
	}
	count := cursorLine.Number()
	return Range{Path: cursorLine.Path, Side: cursorLine.Side, Start: count, End: count}, true
}

// cursorComment is the index of the comment whose rows the cursor is on, from
// the owners stored when the thread's block was built.
func (model *Model) cursorComment() (commentIndex int, ok bool) {
	cursor := model.window.Cursor()
	if cursor >= len(model.rows) || model.rows[cursor].threadRow == nil {
		return 0, false
	}
	placement := *model.rows[cursor].threadRow
	// SetThreadExpanded changes the fold state without a relayout, so the
	// stored owners can describe a layout that is no longer the chosen one.
	if !model.expanded(model.fileThreads[placement.threadIndex]) {
		return 0, false
	}
	owners := model.owners[placement.threadIndex]
	if placement.rowInBlock >= len(owners) || owners[placement.rowInBlock] < 0 {
		return 0, false
	}
	return owners[placement.rowInBlock], true
}
