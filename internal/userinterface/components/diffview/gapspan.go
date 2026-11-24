package diffview

import "github.com/MikePors/strigidae/internal/diff"

// gapSpan is where the lines of one gap are in the new side of the file.
type gapSpan struct {
	first, last int // line numbers; empty when last < first
	offset      int // a line's old number is its new number plus offset
	known       bool
}

func (span gapSpan) size() int { return max(span.last-span.first+1, 0) }

// The first and last line a hunk covers on each side. A side with no lines in
// the hunk (a pure insertion or deletion) sits after the line its Start names.
func newBegin(hunk diff.Hunk) int {
	if hunk.NewLines == 0 {
		return hunk.NewStart + 1
	}
	return hunk.NewStart
}

func newEnd(hunk diff.Hunk) int {
	if hunk.NewLines == 0 {
		return hunk.NewStart
	}
	return hunk.NewStart + hunk.NewLines - 1
}

func oldBegin(hunk diff.Hunk) int {
	if hunk.OldLines == 0 {
		return hunk.OldStart + 1
	}
	return hunk.OldStart
}

func oldEnd(hunk diff.Hunk) int {
	if hunk.OldLines == 0 {
		return hunk.OldStart
	}
	return hunk.OldStart + hunk.OldLines - 1
}

// canExpand reports whether the shown file has gaps to show: it has hunks and
// exists on both sides, so the lines between them are on its new side. An
// added or deleted file's diff holds the whole file.
func canExpand(file *diff.File) bool {
	if file == nil || file.Binary || file.Truncated || len(file.Hunks) == 0 {
		return false
	}
	return file.Status == diff.Modified || file.Status == diff.Renamed || file.Status == diff.Copied
}

// gapSpan returns where gap index is. A gap between hunks is bounded by them;
// the one below the last hunk ends at the end of the file, known only once the
// text is.
func (model *Model) gapSpan(index int) gapSpan {
	hunks := model.file.Hunks
	if index < len(hunks) {
		next := hunks[index]
		first := 1
		if index > 0 {
			first = newEnd(hunks[index-1]) + 1
		}
		last := newBegin(next) - 1
		if model.fileLines != nil {
			last = min(last, len(model.fileLines)) // text that disagrees with the diff
		}
		return gapSpan{first: first, last: last, offset: oldBegin(next) - newBegin(next), known: true}
	}
	previous := hunks[len(hunks)-1]
	span := gapSpan{first: newEnd(previous) + 1, offset: oldEnd(previous) - newEnd(previous)}
	if model.fileLines != nil {
		span.last, span.known = len(model.fileLines), true
	}
	return span
}

// gapState says how much of gap index is shown and how much still hidden
// (-1 when that is not known yet). A file with no gaps has none.
func (model *Model) gapState(index int) (span gapSpan, shown, hidden int) {
	span = model.gapSpan(index)
	if !span.known {
		return span, 0, -1
	}
	shown = min(model.revealed[index], span.size())
	return span, shown, span.size() - shown
}

// gapHunk is the hunk the rows of gap index belong to: the one whose lines they
// continue.
func gapHunk(index int) int { return max(index-1, 0) }
