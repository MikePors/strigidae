// Package diffview renders the unified diff of ONE file: old and new line
// number gutters, add/delete colouring, syntax highlighting, a cursor line,
// vertical and horizontal scrolling.
//
// It is driven by its host through methods, never through key messages. A
// workspace maps keys to methods like this:
//
//	j / k          CursorDown(1) / CursorUp(1)
//	g g / G        GotoTop() / GotoBottom()
//	ctrl+d / ctrl+u HalfPageDown() / HalfPageUp()
//	ctrl+f / ctrl+b PageDown() / PageUp()
//	] h / [ h      NextHunk() / PrevHunk()
//	z h / z l      ScrollLeft(4) / ScrollRight(4)
//	z H / z L      HalfScrollLeft() / HalfScrollRight()
//	z a / z o / z c  ToggleThread() / OpenThread() / CloseThread()
//	] c / [ c      NextThread() / PrevThread()
//
// Usage:
//
//	dv := diffview.New(theme.Default())
//	dv.SetFile(parsed.File("main.go"))
//	out := dv.Render(width, height)
//	if c, ok := dv.Cursor(); ok { /* comment on c.Path line c.New or c.Old */ }
//
// Review threads (SetThreads) are placed under the line they are anchored to
// and take extra rows: one header row, plus the comments while expanded.
// Threads that no longer map onto a line of the diff are listed after the last
// row. Only the visible rows are ever styled or highlighted, and highlighted
// rows are cached, so scrolling a 5k-line file never re-lexes it.
package diffview

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
	"github.com/MikePors/strigidae/internal/ui/components/scroll"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// Side says which side of the diff a cursor line belongs to, in the sense
// GitHub uses for review comments.
type Side int

// Sides.
const (
	// SideRight is the new file: added and unchanged lines.
	SideRight Side = iota
	// SideLeft is the old file: deleted lines.
	SideLeft
)

// CursorLine describes the diff line under the cursor.
type CursorLine struct {
	Path     string // the file's current path (old path for deleted files)
	Hunk     int    // index into File.Hunks
	Kind     diff.LineKind
	Old, New int // line numbers; 0 on the side where the line does not exist
	Side     Side
	Text     string
}

// Number returns the line number on the cursor's side.
func (c CursorLine) Number() int {
	if c.Side == SideLeft {
		return c.Old
	}
	return c.New
}

// Option configures New.
type Option func(*Model)

// WithHighlighter replaces the default Chroma highlighter (used by tests to
// count calls or to keep output free of lexer details).
func WithHighlighter(h Highlighter) Option { return func(m *Model) { m.highlight = h } }

// WithClock sets the clock used for the relative times in review threads.
func WithClock(now func() time.Time) Option { return func(m *Model) { m.now = now } }

// row is one display row: a hunk header (line == nil, tr == nil), a diff line,
// or one line of a review thread (tr != nil).
type row struct {
	hunk int
	line *diff.Line
	text string // display text: tabs expanded, controls replaced
	bi   int    // index in base of this row, or of the row a thread row follows
	tr   *threadRow
	msg  bool // one line of the file's notice (binary, renamed, ...), shown above its threads
}

// threadRow places a row inside a thread's block of lines.
type threadRow struct{ t, i int }

// Model is the diff pane. Create it with New. It is not a tea.Model.
type Model struct {
	th        theme.Theme
	highlight Highlighter
	styles    styleSet

	file    *diff.File
	msg     string // when set, shown instead of the diff
	base    []row  // the diff's own rows; rows is base with thread rows added
	rows    []row
	hunkRow []int // row index of each hunk header

	now      func() time.Time
	md       *markdown.Renderer
	threads  []github.ReviewThread // all of the pull request's threads
	fthreads []github.ReviewThread // the ones on the shown file
	attach   map[int][]int         // base row -> indexes in fthreads
	orphans  []int                 // fthreads that map to no diff line
	open     map[string]bool       // thread ID -> expanded, once the user chose
	blocks   [][]string            // rendered lines of each fthreads entry
	bcache   map[blockKey][]string // rendered threads; dropped by SetThreads and when width or theme change
	bsig     blockSig
	renders  int // thread blocks rendered, for tests
	numW     int // width of one line-number column
	maxW     int // widest code line, in cells

	cache map[int][]Span // highlighted rows, by base row index; reset per file
	saved *position      // position kept while a message replaces the file

	w, h int

	win scroll.Window // cursor, top row and horizontal offset
}

// New returns an empty diff pane.
func New(th theme.Theme, opts ...Option) *Model {
	m := &Model{th: th, highlight: ChromaHighlighter(), cache: map[int][]Span{}, now: time.Now, open: map[string]bool{}}
	for _, o := range opts {
		o(m)
	}
	m.styles = newStyleSet(th)
	m.md = markdown.New(th)
	return m
}

// SetTheme swaps the theme. The highlight cache holds token classes, not
// colours, so it stays valid.
func (m *Model) SetTheme(th theme.Theme) {
	m.th = th
	m.styles = newStyleSet(th)
	m.md = markdown.New(th)
	m.layout()
}

// SetFile shows f (nil clears the pane). Passing the file already shown is a
// no-op. Passing a different *diff.File with the same path (a re-parsed
// refresh) rebuilds the rows and re-anchors the cursor on the same diff line
// (matching kind and line numbers, else the nearest line), keeping its place in
// the window and the horizontal offset. A different path resets them.
func (m *Model) SetFile(f *diff.File) {
	if f != nil && f == m.file && m.msg == "" {
		return
	}
	// A refresh re-parses the diff, giving a new pointer for the same file
	// (possibly with a message shown in between): put the cursor back on the
	// same diff line so the reviewer does not lose their place.
	var prev *position
	switch {
	case f == nil:
	case m.file != nil && f.Path() == m.file.Path():
		p := m.capture()
		prev = &p
	case m.file == nil && m.saved != nil && m.saved.path == f.Path():
		prev = m.saved
	}
	m.saved = nil
	m.file, m.msg = f, ""
	m.reset()
	if f == nil {
		return
	}
	if f.Binary || len(f.Hunks) == 0 {
		// No diff rows: the file's notice and its threads are all there is.
		m.placeThreads()
		m.layout()
	} else {
		m.build(f)
	}
	if prev != nil {
		m.restore(*prev)
	}
}

// SetMessage shows a message instead of a diff (for example while loading).
// The position in the file being replaced is remembered, so a following
// SetFile for the same path (the refresh finishing) restores it.
func (m *Model) SetMessage(msg string) {
	if m.file != nil {
		p := m.capture()
		m.saved = &p
	}
	m.file, m.msg = nil, msg
	m.reset()
}

// SetDiffError shows the message for a failed diff load; it recognises
// github.ErrDiffTooLarge.
func (m *Model) SetDiffError(err error) {
	if errors.Is(err, github.ErrDiffTooLarge) {
		m.SetMessage(TooLargeMessage())
		return
	}
	m.SetMessage("Could not load the diff: " + err.Error())
}

// TooLargeMessage is the text shown when GitHub refuses to serve the diff.
func TooLargeMessage() string {
	return "This diff is too large to load through the GitHub API.\nOpen the pull request in the browser to view it."
}

func (m *Model) reset() {
	m.bcache = nil
	m.base, m.rows, m.hunkRow = nil, nil, nil
	m.fthreads, m.attach, m.orphans, m.blocks = nil, nil, nil, nil
	m.cache = map[int][]Span{}
	m.win.Reset()
	m.maxW, m.numW = 0, 0
	m.sync()
}

func (m *Model) build(f *diff.File) {
	total := 0
	for _, h := range f.Hunks {
		total += len(h.Lines) + 1
	}
	m.base = make([]row, 0, total)
	maxNum := 0
	for hi := range f.Hunks {
		h := &f.Hunks[hi]
		m.base = append(m.base, row{hunk: hi, bi: len(m.base)})
		maxNum = max(maxNum, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
		for li := range h.Lines {
			l := &h.Lines[li]
			text, w := displayText(l.Text)
			m.maxW = max(m.maxW, w)
			m.base = append(m.base, row{hunk: hi, line: l, text: text, bi: len(m.base)})
		}
	}
	m.numW = max(len(strconv.Itoa(maxNum)), 3)
	m.placeThreads()
	m.layout()
}

// SetSize sets the drawing area. Scroll positions are re-clamped.
func (m *Model) SetSize(w, h int) {
	relayout := w != m.w
	m.w, m.h = max(w, 0), max(h, 0)
	if relayout {
		m.layout() // thread text wraps to the width
		return
	}
	m.sync()
}

// Size returns the current drawing area.
func (m *Model) Size() (w, h int) { return m.w, m.h }

// gutterW is the fixed width of the non-scrolling left part of a row:
// cursor mark, old number, new number, thread marker, change marker.
func (m *Model) gutterW() int { return 2*m.numW + 5 }

func (m *Model) codeW() int { return max(m.w-m.gutterW(), 0) }

// sync pushes the diff's row count and widths into the scroll window.
func (m *Model) sync() {
	m.win.SetRows(len(m.rows))
	m.win.SetSize(m.h)
	m.win.SetWidths(m.maxW, m.codeW())
}

// cursorKey identifies the cursor row independently of how many thread rows
// precede it. A thread row is identified by the thread's ID, so a reload that
// adds or removes threads keeps the cursor on the same one.
type cursorKey struct {
	bi     int
	thread string // ID of the thread the cursor is in, or ""
	off    int    // row within the thread's block
}

func (m *Model) cursorKey() (cursorKey, bool) {
	c := m.win.Cursor()
	if c >= len(m.rows) {
		return cursorKey{}, false
	}
	r := m.rows[c]
	k := cursorKey{bi: r.bi}
	if r.tr != nil {
		k.thread, k.off = m.fthreads[r.tr.t].ID, r.tr.i
	}
	return k, true
}

// keyRow returns the row for k; a thread row that is gone (a collapsed block
// is shorter) falls back to the nearest one in its block, then to its line.
func (m *Model) keyRow(k cursorKey) int {
	if k.thread != "" {
		if r := m.threadRowFor(k.thread, k.off); r >= 0 {
			return r
		}
	}
	for i, r := range m.rows {
		if r.tr == nil && r.bi == k.bi {
			return i
		}
	}
	return 0
}

// threadRowFor returns the row at offset off in the block of the thread with
// the given ID (clamped to the block), or -1 if the thread is not shown.
func (m *Model) threadRowFor(id string, off int) int {
	best := -1
	for i, r := range m.rows {
		if r.tr != nil && m.fthreads[r.tr.t].ID == id && r.tr.i <= off {
			best = i
		}
	}
	return best
}

// layout rebuilds rows from the diff's rows and the threads, keeping the
// cursor on the same row and at the same place in the view.
func (m *Model) layout() {
	k, had := m.cursorKey()
	m.relayout(k, had)
}

// relayout is layout with the cursor key taken by the caller, for changes to
// the thread list, which would make the current rows' thread indexes stale.
func (m *Model) relayout(k cursorKey, had bool) {
	off := m.win.Cursor() - m.win.Top()
	m.rows = m.rows[:0]
	m.hunkRow = m.hunkRow[:0]
	m.blocks = make([][]string, len(m.fthreads))
	for i := range m.fthreads {
		m.blocks[i] = m.renderBlock(i)
	}
	base := m.base
	if m.file != nil && len(base) == 0 && len(m.fthreads) > 0 {
		base = m.noticeRows()
	}
	var pending []int
	flush := func(bi int) {
		for _, ti := range pending {
			for li := range m.blocks[ti] {
				m.rows = append(m.rows, row{bi: bi, tr: &threadRow{t: ti, i: li}})
			}
		}
		pending = pending[:0]
	}
	for i, r := range base {
		if r.line == nil && !r.msg {
			m.hunkRow = append(m.hunkRow, len(m.rows))
		}
		m.rows = append(m.rows, r)
		pending = append(pending, m.attach[i]...)
		if i+1 < len(base) && base[i+1].line != nil && base[i+1].line.Kind == diff.NoNewline {
			continue // the marker belongs to this line; threads go after it
		}
		flush(i)
	}
	pending = append(pending, m.orphans...)
	flush(len(base) - 1)
	m.sync()
	if had && len(m.rows) > 0 {
		c := m.keyRow(k)
		m.win.Restore(c, c-off, m.win.XOffset())
	}
}

// --- vertical movement: cursor follows, view follows the cursor ---

// CursorDown moves the cursor down n rows, scrolling to keep it visible.
func (m *Model) CursorDown(n int) { m.win.Down(n) }

// CursorUp moves the cursor up n rows, scrolling to keep it visible.
func (m *Model) CursorUp(n int) { m.win.Up(n) }

// GotoTop moves the cursor to the first row.
func (m *Model) GotoTop() { m.win.GotoTop() }

// GotoBottom moves the cursor to the last row.
func (m *Model) GotoBottom() { m.win.GotoBottom() }

// --- viewport movement: view and cursor move together, as in vim ---

// ScrollDown scrolls the view down n rows without moving the cursor, unless
// the cursor would leave the view, in which case it is pulled along.
func (m *Model) ScrollDown(n int) { m.win.ScrollDown(n) }

// ScrollUp is the opposite of ScrollDown.
func (m *Model) ScrollUp(n int) { m.win.ScrollUp(n) }

// HalfPageDown moves the view and the cursor down half a screen.
func (m *Model) HalfPageDown() { m.win.HalfPageDown() }

// HalfPageUp moves the view and the cursor up half a screen.
func (m *Model) HalfPageUp() { m.win.HalfPageUp() }

// PageDown moves the view and the cursor down a full screen.
func (m *Model) PageDown() { m.win.PageDown() }

// PageUp moves the view and the cursor up a full screen.
func (m *Model) PageUp() { m.win.PageUp() }

// NextHunk moves the cursor to the next hunk header (no-op on the last hunk),
// putting the header at the top of the view.
func (m *Model) NextHunk() {
	for _, r := range m.hunkRow {
		if r > m.win.Cursor() {
			m.win.JumpTo(r)
			return
		}
	}
}

// PrevHunk moves the cursor to the closest hunk header above it, which is the
// current hunk's own header when the cursor is inside the hunk.
func (m *Model) PrevHunk() {
	for i := len(m.hunkRow) - 1; i >= 0; i-- {
		if m.hunkRow[i] < m.win.Cursor() {
			m.win.JumpTo(m.hunkRow[i])
			return
		}
	}
}

// --- horizontal scrolling ---

// ScrollRight shifts the code area n cells to the right (reveals later text).
func (m *Model) ScrollRight(n int) { m.win.Right(n) }

// ScrollLeft shifts the code area n cells to the left.
func (m *Model) ScrollLeft(n int) { m.win.Left(n) }

// HalfScrollRight scrolls right by half the code area.
func (m *Model) HalfScrollRight() { m.win.HalfRight() }

// HalfScrollLeft scrolls left by half the code area.
func (m *Model) HalfScrollLeft() { m.win.HalfLeft() }

// XOffset returns the horizontal scroll in cells.
func (m *Model) XOffset() int { return m.win.XOffset() }

// Top returns the index of the first visible row (hunk headers count as rows).
func (m *Model) Top() int { return m.win.Top() }

// Cursor describes the diff line under the cursor. ok is false when there is
// no file, or the cursor is on a hunk header or a "no newline" marker.
func (m *Model) Cursor() (c CursorLine, ok bool) {
	if m.file == nil || m.win.Cursor() >= len(m.rows) {
		return CursorLine{}, false
	}
	r := m.rows[m.win.Cursor()]
	if r.line == nil || r.line.Kind == diff.NoNewline {
		return CursorLine{}, false
	}
	c = CursorLine{
		Path: m.file.Path(), Hunk: r.hunk, Kind: r.line.Kind,
		Old: r.line.Old, New: r.line.New, Text: r.line.Text,
	}
	if r.line.Kind == diff.Delete {
		c.Side = SideLeft
	}
	return c, true
}

// Render draws the pane in a w x h area. It is SetSize followed by the render,
// so hosts can call it from their own Render(width, height).
func (m *Model) Render(w, h int) string {
	m.SetSize(w, h)
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if len(m.rows) == 0 {
		return m.renderMessage()
	}
	end := min(m.win.Top()+m.h, len(m.rows))
	lines := make([]string, 0, end-m.win.Top())
	for i := m.win.Top(); i < end; i++ {
		lines = append(lines, m.renderRow(i))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderMessage() string {
	lines := m.messageLines()
	var out []string
	for _, l := range lines {
		l, _ = displayText(l)
		out = append(out, strings.Split(ansi.Wrap(l, m.w, ""), "\n")...)
	}
	if len(out) > m.h {
		out = out[:m.h]
	}
	for i, l := range out {
		out[i] = m.th.Muted.Render(l)
	}
	// Anything built from diff content (paths, modes, errors) is untrusted.
	return strings.Join(out, "\n")
}

func (m *Model) messageLines() []string {
	switch {
	case m.msg != "":
		return strings.Split(m.msg, "\n")
	case m.file == nil:
		return []string{"No file selected."}
	}
	f := m.file
	var out []string
	if f.Binary {
		out = append(out, "Binary file: contents not shown.")
	}
	switch f.Status {
	case diff.Renamed:
		out = append(out, "Renamed: "+f.OldPath+" -> "+f.NewPath+similarity(f))
	case diff.Copied:
		out = append(out, "Copied: "+f.OldPath+" -> "+f.NewPath+similarity(f))
	}
	if f.ModeChanged() {
		out = append(out, "Mode changed: "+f.OldMode+" -> "+f.NewMode)
	}
	if len(out) == 0 {
		switch f.Status {
		case diff.Added:
			out = append(out, "Empty file added.")
		case diff.Deleted:
			out = append(out, "Empty file deleted.")
		default:
			out = append(out, "No changes to show.")
		}
	}
	return out
}

func similarity(f *diff.File) string {
	if f.Similarity <= 0 {
		return ""
	}
	return " (" + strconv.Itoa(f.Similarity) + "% similar)"
}
