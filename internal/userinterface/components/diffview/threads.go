package diffview

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/thread"
)

// Glyphs in the gutter column that marks commented lines.
const (
	markerOpen  = "●" // at least one thread that is neither resolved nor outdated
	markerQuiet = "○" // only resolved or outdated threads
)

// SetThreads gives the pull request's review threads; the pane shows the ones
// on the file it displays. Which threads the user expanded or collapsed is
// kept across calls, by thread ID.
func (model *Model) SetThreads(threads []github.ReviewThread) {
	key, found := model.cursorKey()
	model.threads = threads
	model.blockCache = nil
	model.placeThreads()
	model.relayout(key, found)
}

// placeThreads picks the threads on the shown file and anchors each to a row
// of the diff. A thread goes after the row whose line number on the thread's
// side equals Line. One with no current line (outdated, or on the whole file)
// or whose line is not part of the diff goes in the list after the last row.
func (model *Model) placeThreads() {
	model.fthreads, model.attach, model.orphans = nil, nil, nil
	if model.file == nil {
		return
	}
	byNew, byOld := map[int]int{}, map[int]int{}
	for i, row := range model.base {
		if !row.isLine() {
			continue
		}
		if row.line.New > 0 {
			byNew[row.line.New] = i
		}
		if row.line.Old > 0 {
			byOld[row.line.Old] = i
		}
	}
	path := model.file.Path()
	for _, thread := range model.threads {
		if thread.Path != path {
			continue
		}
		threadIndex := len(model.fthreads)
		model.fthreads = append(model.fthreads, thread)
		lines := byNew
		if thread.DiffSide == github.SideLeft {
			lines = byOld
		}
		if baseIndex, ok := lines[thread.Line]; ok && thread.Line > 0 {
			if model.attach == nil {
				model.attach = map[int][]int{}
			}
			model.attach[baseIndex] = append(model.attach[baseIndex], threadIndex)
			continue
		}
		model.orphans = append(model.orphans, threadIndex)
	}
}

func (model *Model) expanded(reviewThread github.ReviewThread) bool {
	if value, ok := model.open[reviewThread.Identifier]; ok {
		return value
	}
	return thread.DefaultExpanded(reviewThread)
}

// blockSignature is what every cached block depends on besides its own key.
type blockSignature struct {
	width           int
	themeIdentifier string
	minute          time.Time // relative times age
}

type blockKey struct {
	identifier string
	expanded   bool
	location   bool
}

// renderBlock returns the lines of thread ti, from the cache when the thread,
// its expanded state, the width and the theme are unchanged.
func (model *Model) renderBlock(threadIndex int) []string {
	reviewThread := model.fthreads[threadIndex]
	now := model.now()
	signature := blockSignature{width: model.width - model.gutterWidth(), themeIdentifier: model.theme.Identifier, minute: now.Truncate(time.Minute)}
	if model.blockCache == nil || signature != model.blockSignature {
		model.blockCache, model.blockSignature = map[blockKey][]string{}, signature
	}
	key := blockKey{identifier: reviewThread.Identifier, expanded: model.expanded(reviewThread), location: slices.Contains(model.orphans, threadIndex)}
	if lines, ok := model.blockCache[key]; ok {
		return lines
	}
	model.renders++
	environment := thread.Environment{Theme: model.theme, Markdown: model.markdown, Now: now}
	lines := thread.Lines(environment, reviewThread, signature.width, thread.Options{Expanded: key.expanded, Location: key.location})
	model.blockCache[key] = lines
	return lines
}

// noticeRows are the rows for the notice of a file with no diff rows (binary,
// rename only, ...), wrapped to the width, so its threads can follow it.
func (model *Model) noticeRows() []row {
	var out []row
	for _, line := range model.messageLines() {
		line, _ = displayText(line)
		for _, word := range strings.Split(ansi.Wrap(line, max(model.width-1, 1), ""), "\n") {
			out = append(out, row{message: true, text: word, baseIndex: len(out)})
		}
	}
	return out
}

func (model *Model) renderNoticeRow(i int, row row) string {
	lead := " "
	if i == model.window.Cursor() {
		lead = ">"
	}
	text := ansi.Truncate(lead+row.text, model.width, "")
	return model.theme.Muted.Render(text + spaces(model.width-ansi.StringWidth(text)))
}

// threadsAtCursor returns the threads the cursor is on: the one whose rows it
// is in, or those anchored to the diff line it is on.
func (model *Model) threadsAtCursor() []int {
	count := model.window.Cursor()
	if count >= len(model.rows) {
		return nil
	}
	if row := model.rows[count]; row.threadRow != nil {
		return []int{row.threadRow.threadIndex}
	}
	return model.attach[model.rows[count].baseIndex]
}

// ThreadsAtCursor returns the threads the cursor is on: the one whose rows it
// is in, or those anchored to the diff line it is on.
func (model *Model) ThreadsAtCursor() []github.ReviewThread {
	var out []github.ReviewThread
	for _, threadIndex := range model.threadsAtCursor() {
		out = append(out, model.fthreads[threadIndex])
	}
	return out
}

func (model *Model) setExpanded(timestamps []int, value bool) {
	if len(timestamps) == 0 {
		return
	}
	for _, threadIndex := range timestamps {
		model.open[model.fthreads[threadIndex].Identifier] = value
	}
	model.layout()
}

// OpenThread expands the thread under the cursor (every thread on the line
// when the cursor is on a diff line).
func (model *Model) OpenThread() { model.setExpanded(model.threadsAtCursor(), true) }

// CloseThread collapses the thread under the cursor. The cursor ends on its
// header row.
func (model *Model) CloseThread() { model.setExpanded(model.threadsAtCursor(), false) }

// ToggleThread collapses the thread under the cursor if every one there is
// expanded, else expands them.
func (model *Model) ToggleThread() {
	timestamps := model.threadsAtCursor()
	all := len(timestamps) > 0
	for _, threadIndex := range timestamps {
		all = all && model.expanded(model.fthreads[threadIndex])
	}
	model.setExpanded(timestamps, !all)
}

// NextThread moves the cursor to the header of the next thread below it (no-op
// after the last one), putting it at the top of the view.
func (model *Model) NextThread() {
	for i := model.window.Cursor() + 1; i < len(model.rows); i++ {
		if row := model.rows[i]; row.threadRow != nil && row.threadRow.i == 0 {
			model.window.JumpTo(i)
			return
		}
	}
}

// PreviousThread moves the cursor to the header of the closest thread above it.
func (model *Model) PreviousThread() {
	for i := min(model.window.Cursor(), len(model.rows)) - 1; i >= 0; i-- {
		if row := model.rows[i]; row.threadRow != nil && row.threadRow.i == 0 {
			model.window.JumpTo(i)
			return
		}
	}
}

// ThreadCount is the number of threads shown for the current file, in place
// or in the list after the diff.
func (model *Model) ThreadCount() int { return len(model.fthreads) }

// threadGlyph returns the gutter marker for base row bi and whether it is the
// "open" kind.
func (model *Model) threadGlyph(baseIndex int) (glyph string, open bool) {
	timestamps := model.attach[baseIndex]
	if len(timestamps) == 0 {
		return " ", false
	}
	for _, threadIndex := range timestamps {
		if !thread.Quiet(model.fthreads[threadIndex]) {
			return markerOpen, true
		}
	}
	return markerQuiet, false
}

func (model *Model) renderThreadRow(i int, row row) string {
	lead := " "
	if i == model.window.Cursor() {
		lead = ">"
	}
	lead += spaces(model.gutterWidth() - 1)
	text := ansi.Truncate(lead+model.blocks[row.threadRow.threadIndex][row.threadRow.i], model.width, "")
	return text + spaces(model.width-ansi.StringWidth(text))
}
