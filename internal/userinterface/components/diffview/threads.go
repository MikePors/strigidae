package diffview

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/components/thread"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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

// SetThreadExpanded folds the thread with the given identifier open or closed,
// replacing what the user chose before. The rows are laid out again at once,
// so the cursor context always matches what is drawn; the caller's later
// SetThreads keeps the choice.
func (model *Model) SetThreadExpanded(identifier string, expanded bool) {
	model.open[identifier] = expanded
	model.layout()
}

// placeThreads picks the threads on the shown file and anchors each to a row
// of the diff. A thread goes after the row whose line number on the thread's
// side equals Line. One with no current line (outdated, or on the whole file)
// or whose line is not part of the diff goes in the list after the last row.
func (model *Model) placeThreads() {
	model.fileThreads, model.attach, model.orphans = nil, nil, nil
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
		threadIndex := len(model.fileThreads)
		model.fileThreads = append(model.fileThreads, thread)
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

// SetHideResolved leaves resolved threads out of the pane, or shows them
// again. A line whose only threads are hidden keeps its gutter marker, but
// the cursor cannot reach the threads: NextThread, PreviousThread and the
// fold and cursor-thread methods skip them. A cursor inside a thread that
// leaves moves to its line.
func (model *Model) SetHideResolved(hide bool) {
	if hide == model.hideResolved {
		return
	}
	model.hideResolved = hide
	model.layout()
}

// shownThreads returns the threads of threadIndexes that are not hidden.
func (model *Model) shownThreads(threadIndexes []int) []int {
	if !model.hideResolved {
		return threadIndexes
	}
	var shown []int
	for _, threadIndex := range threadIndexes {
		if !model.threadHidden(model.fileThreads[threadIndex]) {
			shown = append(shown, threadIndex)
		}
	}
	return shown
}

// SetThreadFade sets how the pane learns whether a thread is fading out and
// how far: step is 0 for its normal look, up to theme.FadeSteps for the
// faintest. A resolved thread that is fading stays shown under
// SetHideResolved until fading turns false and ThreadFadesChanged is called.
// The step is asked while rendering. nil fades none.
func (model *Model) SetThreadFade(fade func(identifier string) (step int, fading bool)) {
	model.threadFade = fade
}

// ThreadFadesChanged rebuilds the rows after a thread stopped fading, so a
// resolved one leaves under SetHideResolved. A cursor inside it moves to its
// line.
func (model *Model) ThreadFadesChanged() { model.layout() }

func (model *Model) threadHidden(reviewThread github.ReviewThread) bool {
	return model.hideResolved && reviewThread.IsResolved && !model.threadFading(reviewThread.Identifier)
}

func (model *Model) threadFading(identifier string) bool {
	if model.threadFade == nil {
		return false
	}
	_, fading := model.threadFade(identifier)
	return fading
}

// threadFadeStep returns how far thread threadIndex has faded.
func (model *Model) threadFadeStep(threadIndex int) int {
	if model.threadFade == nil {
		return 0
	}
	step, _ := model.threadFade(model.fileThreads[threadIndex].Identifier)
	return min(step, theme.FadeSteps)
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

// block is one laid-out thread: its lines and, parallel to them, the index in
// the thread's Comments each belongs to (-1 for none).
type block struct {
	lines  []string
	owners []int
}

// renderBlock returns the lines of thread threadIndex, from the cache when the thread,
// its expanded state, the width and the theme are unchanged.
func (model *Model) renderBlock(threadIndex int) block {
	reviewThread := model.fileThreads[threadIndex]
	now := model.now()
	signature := blockSignature{width: model.width - model.gutterWidth(), themeIdentifier: model.theme.Identifier, minute: now.Truncate(time.Minute)}
	if model.blockCache == nil || signature != model.blockSignature {
		model.blockCache, model.blockSignature = map[blockKey]block{}, signature
	}
	key := blockKey{identifier: reviewThread.Identifier, expanded: model.expanded(reviewThread), location: slices.Contains(model.orphans, threadIndex)}
	if cached, ok := model.blockCache[key]; ok {
		return cached
	}
	model.renders++
	environment := thread.Environment{Theme: model.theme, Markdown: model.markdownRenderer, Now: now}
	lines, owners := thread.Layout(environment, reviewThread, signature.width, thread.Options{Expanded: key.expanded, Location: key.location})
	built := block{lines: lines, owners: owners}
	model.blockCache[key] = built
	return built
}

// noticeRows are the rows for the notice of a file with no diff rows (binary,
// rename only, ...), wrapped to the width, so its threads can follow it.
func (model *Model) noticeRows() []row {
	var out []row
	for _, line := range model.messageLines() {
		line, _ = displayText(line)
		for _, wrapped := range strings.Split(ansi.Wrap(line, max(model.width-1-model.relativeColumnWidth(), 1), ""), "\n") {
			out = append(out, row{message: true, text: wrapped, baseIndex: len(out)})
		}
	}
	return out
}

func (model *Model) renderNoticeRow(i int, row row) string {
	lead := " "
	if i == model.window.Cursor() {
		lead = ">"
	}
	text := ansi.Truncate(lead+model.relativeCell(i)+row.text, model.width, "")
	return model.theme.Muted.Render(text + spaces(model.width-ansi.StringWidth(text)))
}

// threadsAtCursor returns the threads the cursor is on: the one whose rows it
// is in, or those anchored to the diff line it is on.
func (model *Model) threadsAtCursor() []int {
	cursor := model.window.Cursor()
	if cursor >= len(model.rows) {
		return nil
	}
	if row := model.rows[cursor]; row.threadRow != nil {
		return []int{row.threadRow.threadIndex}
	}
	return model.shownThreads(model.attach[model.rows[cursor].baseIndex])
}

func (model *Model) setExpanded(threadIndexes []int, value bool) {
	if len(threadIndexes) == 0 {
		return
	}
	for _, threadIndex := range threadIndexes {
		model.open[model.fileThreads[threadIndex].Identifier] = value
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
		all = all && model.expanded(model.fileThreads[threadIndex])
	}
	model.setExpanded(timestamps, !all)
}

// SetSkipResolved makes NextThread and PreviousThread pass over resolved
// threads, or visit them again. Resolved threads stay shown and reachable
// with the cursor keys.
func (model *Model) SetSkipResolved(skip bool) { model.skipResolved = skip }

// threadHeaderAt reports whether row i is the first row of a thread the
// thread keys may land on.
func (model *Model) threadHeaderAt(i int) bool {
	row := model.rows[i]
	if row.threadRow == nil || row.threadRow.rowInBlock != 0 {
		return false
	}
	return !model.skipResolved || !model.fileThreads[row.threadRow.threadIndex].IsResolved
}

// NoThreadToVisit reports whether the file has threads shown but the thread
// keys have none to land on in the given direction, which only happens under
// SetSkipResolved. A pane with no file, a message or no threads reports false:
// there the keys have nothing to say.
func (model *Model) NoThreadToVisit(forward bool) bool {
	if !model.skipResolved || model.file == nil || model.ThreadCount() == 0 {
		return false
	}
	if forward {
		for i := model.window.Cursor() + 1; i < len(model.rows); i++ {
			if model.threadHeaderAt(i) {
				return false
			}
		}
		return true
	}
	for i := min(model.window.Cursor(), len(model.rows)) - 1; i >= 0; i-- {
		if model.threadHeaderAt(i) {
			return false
		}
	}
	return true
}

// NextThread moves the cursor to the header of the next thread below it (no-op
// after the last one), putting it at the top of the view. Under
// SetSkipResolved resolved threads are passed over.
func (model *Model) NextThread() {
	for i := model.window.Cursor() + 1; i < len(model.rows); i++ {
		if model.threadHeaderAt(i) {
			model.window.JumpTo(i)
			return
		}
	}
}

// PreviousThread moves the cursor to the header of the closest thread above
// it, passing over resolved ones under SetSkipResolved.
func (model *Model) PreviousThread() {
	for i := min(model.window.Cursor(), len(model.rows)) - 1; i >= 0; i-- {
		if model.threadHeaderAt(i) {
			model.window.JumpTo(i)
			return
		}
	}
}

// ThreadCount is the number of threads shown for the current file, in place
// or in the list after the diff; hidden resolved ones are not counted.
func (model *Model) ThreadCount() int {
	count := 0
	for _, reviewThread := range model.fileThreads {
		if !model.threadHidden(reviewThread) {
			count++
		}
	}
	return count
}

// threadGlyph returns the gutter marker for base row baseIndex and whether it is the
// "open" kind.
func (model *Model) threadGlyph(baseIndex int) (glyph string, open bool) {
	timestamps := model.attach[baseIndex]
	if len(timestamps) == 0 {
		return " ", false
	}
	for _, threadIndex := range timestamps {
		if !thread.Quiet(model.fileThreads[threadIndex]) {
			return markerOpen, true
		}
	}
	return markerQuiet, false
}

func (model *Model) renderThreadRow(i int, row row) string {
	lead := " "
	switch {
	case i == model.window.Cursor():
		lead = ">"
	case model.searchMarked(i):
		lead = model.theme.SearchMarker.Render(search.Glyph)
	}
	lead += model.relativeCell(i) + spaces(model.lineNumbersWidth())
	text := ansi.Truncate(lead+model.blocks[row.threadRow.threadIndex][row.threadRow.rowInBlock], model.width, "")
	return text + spaces(model.width-ansi.StringWidth(text))
}
