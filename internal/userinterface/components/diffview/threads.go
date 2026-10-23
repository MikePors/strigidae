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
func (m *Model) SetThreads(ts []github.ReviewThread) {
	k, had := m.cursorKey()
	m.threads = ts
	m.bcache = nil
	m.placeThreads()
	m.relayout(k, had)
}

// placeThreads picks the threads on the shown file and anchors each to a row
// of the diff. A thread goes after the row whose line number on the thread's
// side equals Line. One with no current line (outdated, or on the whole file)
// or whose line is not part of the diff goes in the list after the last row.
func (m *Model) placeThreads() {
	m.fthreads, m.attach, m.orphans = nil, nil, nil
	if m.file == nil {
		return
	}
	byNew, byOld := map[int]int{}, map[int]int{}
	for i, r := range m.base {
		if !r.isLine() {
			continue
		}
		if r.line.New > 0 {
			byNew[r.line.New] = i
		}
		if r.line.Old > 0 {
			byOld[r.line.Old] = i
		}
	}
	path := m.file.Path()
	for _, t := range m.threads {
		if t.Path != path {
			continue
		}
		ti := len(m.fthreads)
		m.fthreads = append(m.fthreads, t)
		lines := byNew
		if t.DiffSide == github.SideLeft {
			lines = byOld
		}
		if bi, ok := lines[t.Line]; ok && t.Line > 0 {
			if m.attach == nil {
				m.attach = map[int][]int{}
			}
			m.attach[bi] = append(m.attach[bi], ti)
			continue
		}
		m.orphans = append(m.orphans, ti)
	}
}

func (m *Model) expanded(t github.ReviewThread) bool {
	if v, ok := m.open[t.Identifier]; ok {
		return v
	}
	return thread.DefaultExpanded(t)
}

// blockSig is what every cached block depends on besides its own key.
type blockSig struct {
	width   int
	themeID string
	minute  time.Time // relative times age
}

type blockKey struct {
	id       string
	expanded bool
	location bool
}

// renderBlock returns the lines of thread ti, from the cache when the thread,
// its expanded state, the width and the theme are unchanged.
func (m *Model) renderBlock(ti int) []string {
	t := m.fthreads[ti]
	now := m.now()
	sig := blockSig{width: m.w - m.gutterW(), themeID: m.th.ID, minute: now.Truncate(time.Minute)}
	if m.bcache == nil || sig != m.bsig {
		m.bcache, m.bsig = map[blockKey][]string{}, sig
	}
	key := blockKey{id: t.Identifier, expanded: m.expanded(t), location: slices.Contains(m.orphans, ti)}
	if lines, ok := m.bcache[key]; ok {
		return lines
	}
	m.renders++
	env := thread.Env{Theme: m.th, Markdown: m.md, Now: now}
	lines := thread.Lines(env, t, sig.width, thread.Options{Expanded: key.expanded, Location: key.location})
	m.bcache[key] = lines
	return lines
}

// noticeRows are the rows for the notice of a file with no diff rows (binary,
// rename only, ...), wrapped to the width, so its threads can follow it.
func (m *Model) noticeRows() []row {
	var out []row
	for _, l := range m.messageLines() {
		l, _ = displayText(l)
		for _, w := range strings.Split(ansi.Wrap(l, max(m.w-1, 1), ""), "\n") {
			out = append(out, row{msg: true, text: w, bi: len(out)})
		}
	}
	return out
}

func (m *Model) renderNoticeRow(i int, r row) string {
	lead := " "
	if i == m.win.Cursor() {
		lead = ">"
	}
	s := ansi.Truncate(lead+r.text, m.w, "")
	return m.th.Muted.Render(s + spaces(m.w-ansi.StringWidth(s)))
}

// threadsAtCursor returns the threads the cursor is on: the one whose rows it
// is in, or those anchored to the diff line it is on.
func (m *Model) threadsAtCursor() []int {
	c := m.win.Cursor()
	if c >= len(m.rows) {
		return nil
	}
	if r := m.rows[c]; r.tr != nil {
		return []int{r.tr.t}
	}
	return m.attach[m.rows[c].bi]
}

// ThreadsAtCursor returns the threads the cursor is on: the one whose rows it
// is in, or those anchored to the diff line it is on.
func (m *Model) ThreadsAtCursor() []github.ReviewThread {
	var out []github.ReviewThread
	for _, ti := range m.threadsAtCursor() {
		out = append(out, m.fthreads[ti])
	}
	return out
}

func (m *Model) setExpanded(ts []int, v bool) {
	if len(ts) == 0 {
		return
	}
	for _, ti := range ts {
		m.open[m.fthreads[ti].Identifier] = v
	}
	m.layout()
}

// OpenThread expands the thread under the cursor (every thread on the line
// when the cursor is on a diff line).
func (m *Model) OpenThread() { m.setExpanded(m.threadsAtCursor(), true) }

// CloseThread collapses the thread under the cursor. The cursor ends on its
// header row.
func (m *Model) CloseThread() { m.setExpanded(m.threadsAtCursor(), false) }

// ToggleThread collapses the thread under the cursor if every one there is
// expanded, else expands them.
func (m *Model) ToggleThread() {
	ts := m.threadsAtCursor()
	all := len(ts) > 0
	for _, ti := range ts {
		all = all && m.expanded(m.fthreads[ti])
	}
	m.setExpanded(ts, !all)
}

// NextThread moves the cursor to the header of the next thread below it (no-op
// after the last one), putting it at the top of the view.
func (m *Model) NextThread() {
	for i := m.win.Cursor() + 1; i < len(m.rows); i++ {
		if r := m.rows[i]; r.tr != nil && r.tr.i == 0 {
			m.win.JumpTo(i)
			return
		}
	}
}

// PrevThread moves the cursor to the header of the closest thread above it.
func (m *Model) PrevThread() {
	for i := min(m.win.Cursor(), len(m.rows)) - 1; i >= 0; i-- {
		if r := m.rows[i]; r.tr != nil && r.tr.i == 0 {
			m.win.JumpTo(i)
			return
		}
	}
}

// ThreadCount is the number of threads shown for the current file, in place
// or in the list after the diff.
func (m *Model) ThreadCount() int { return len(m.fthreads) }

// threadGlyph returns the gutter marker for base row bi and whether it is the
// "open" kind.
func (m *Model) threadGlyph(bi int) (glyph string, open bool) {
	ts := m.attach[bi]
	if len(ts) == 0 {
		return " ", false
	}
	for _, ti := range ts {
		if !thread.Quiet(m.fthreads[ti]) {
			return markerOpen, true
		}
	}
	return markerQuiet, false
}

func (m *Model) renderThreadRow(i int, r row) string {
	lead := " "
	if i == m.win.Cursor() {
		lead = ">"
	}
	lead += spaces(m.gutterW() - 1)
	s := ansi.Truncate(lead+m.blocks[r.tr.t][r.tr.i], m.w, "")
	return s + spaces(m.w-ansi.StringWidth(s))
}
