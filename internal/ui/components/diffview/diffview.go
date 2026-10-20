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
//
// Usage:
//
//	dv := diffview.New(theme.Default())
//	dv.SetFile(parsed.File("main.go"))
//	out := dv.Render(width, height)
//	if c, ok := dv.Cursor(); ok { /* comment on c.Path line c.New or c.Old */ }
//
// Only the visible rows are ever styled or highlighted, and highlighted rows
// are cached, so scrolling a 5k-line file never re-lexes it.
package diffview

import (
	"errors"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
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

// row is one display row: a hunk header (line == nil) or a diff line.
type row struct {
	hunk int
	line *diff.Line
	text string // display text: tabs expanded, controls replaced
}

// Model is the diff pane. Create it with New. It is not a tea.Model.
type Model struct {
	th        theme.Theme
	highlight Highlighter
	styles    styleSet

	file    *diff.File
	msg     string // when set, shown instead of the diff
	rows    []row
	hunkRow []int // row index of each hunk header
	numW    int   // width of one line-number column
	maxW    int   // widest code line, in cells

	cache map[int][]Span // highlighted rows, by row index; reset per file

	w, h   int
	top    int // first visible row
	cursor int // row index
	xoff   int // horizontal scroll, in cells
}

// New returns an empty diff pane.
func New(th theme.Theme, opts ...Option) *Model {
	m := &Model{th: th, highlight: ChromaHighlighter(), cache: map[int][]Span{}}
	for _, o := range opts {
		o(m)
	}
	m.styles = newStyleSet(th)
	return m
}

// SetTheme swaps the theme. The highlight cache holds token classes, not
// colours, so it stays valid.
func (m *Model) SetTheme(th theme.Theme) {
	m.th = th
	m.styles = newStyleSet(th)
}

// SetFile shows f (nil clears the pane). Passing the file already shown is a
// no-op, so a host can call it on every refresh without losing the scroll
// position. Scroll and cursor reset for a different file.
func (m *Model) SetFile(f *diff.File) {
	if f != nil && f == m.file && m.msg == "" {
		return
	}
	m.file, m.msg = f, ""
	m.reset()
	if f == nil || f.Binary || len(f.Hunks) == 0 {
		return
	}
	m.build(f)
	m.clamp()
}

// SetMessage shows a message instead of a diff (for example while loading).
func (m *Model) SetMessage(msg string) {
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
	m.rows, m.hunkRow = nil, nil
	m.cache = map[int][]Span{}
	m.top, m.cursor, m.xoff, m.maxW, m.numW = 0, 0, 0, 0, 0
}

func (m *Model) build(f *diff.File) {
	total := 0
	for _, h := range f.Hunks {
		total += len(h.Lines) + 1
	}
	m.rows = make([]row, 0, total)
	maxNum := 0
	for hi := range f.Hunks {
		h := &f.Hunks[hi]
		m.hunkRow = append(m.hunkRow, len(m.rows))
		m.rows = append(m.rows, row{hunk: hi})
		maxNum = max(maxNum, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
		for li := range h.Lines {
			l := &h.Lines[li]
			text, w := displayText(l.Text)
			m.maxW = max(m.maxW, w)
			m.rows = append(m.rows, row{hunk: hi, line: l, text: text})
		}
	}
	m.numW = max(len(strconv.Itoa(maxNum)), 3)
}

// SetSize sets the drawing area. Scroll positions are re-clamped.
func (m *Model) SetSize(w, h int) {
	m.w, m.h = max(w, 0), max(h, 0)
	m.clamp()
}

// Size returns the current drawing area.
func (m *Model) Size() (w, h int) { return m.w, m.h }

// gutterW is the fixed width of the non-scrolling left part of a row:
// cursor mark, old number, new number, change marker.
func (m *Model) gutterW() int { return 2*m.numW + 4 }

func (m *Model) codeW() int { return max(m.w-m.gutterW(), 0) }

func (m *Model) maxTop() int { return max(len(m.rows)-m.h, 0) }

func (m *Model) maxX() int { return max(m.maxW-m.codeW(), 0) }

func (m *Model) clamp() {
	if len(m.rows) == 0 {
		m.top, m.cursor, m.xoff = 0, 0, 0
		return
	}
	m.cursor = min(max(m.cursor, 0), len(m.rows)-1)
	m.top = min(max(m.top, 0), m.maxTop())
	m.xoff = min(max(m.xoff, 0), m.maxX())
	m.follow()
}

// follow scrolls the minimum needed to keep the cursor visible.
func (m *Model) follow() {
	if m.h <= 0 {
		return
	}
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+m.h {
		m.top = m.cursor - m.h + 1
	}
}

// --- vertical movement: cursor follows, view follows the cursor ---

// CursorDown moves the cursor down n rows, scrolling to keep it visible.
func (m *Model) CursorDown(n int) { m.cursor += n; m.clamp() }

// CursorUp moves the cursor up n rows, scrolling to keep it visible.
func (m *Model) CursorUp(n int) { m.cursor -= n; m.clamp() }

// GotoTop moves the cursor to the first row.
func (m *Model) GotoTop() { m.cursor = 0; m.clamp() }

// GotoBottom moves the cursor to the last row.
func (m *Model) GotoBottom() { m.cursor = len(m.rows) - 1; m.clamp() }

// --- viewport movement: view and cursor move together, as in vim ---

// ScrollDown scrolls the view down n rows without moving the cursor, unless
// the cursor would leave the view, in which case it is pulled along.
func (m *Model) ScrollDown(n int) { m.scrollBy(n) }

// ScrollUp is the opposite of ScrollDown.
func (m *Model) ScrollUp(n int) { m.scrollBy(-n) }

func (m *Model) scrollBy(n int) {
	m.top = min(max(m.top+n, 0), m.maxTop())
	m.cursor = min(max(m.cursor, m.top), max(m.top+m.h-1, m.top))
	m.clamp()
}

// HalfPageDown moves the view and the cursor down half a screen.
func (m *Model) HalfPageDown() { m.shift(max(m.h/2, 1)) }

// HalfPageUp moves the view and the cursor up half a screen.
func (m *Model) HalfPageUp() { m.shift(-max(m.h/2, 1)) }

// PageDown moves the view and the cursor down a full screen.
func (m *Model) PageDown() { m.shift(max(m.h, 1)) }

// PageUp moves the view and the cursor up a full screen.
func (m *Model) PageUp() { m.shift(-max(m.h, 1)) }

func (m *Model) shift(n int) {
	m.top = min(max(m.top+n, 0), m.maxTop())
	m.cursor += n
	m.clamp()
}

// NextHunk moves the cursor to the next hunk header (no-op on the last hunk).
func (m *Model) NextHunk() {
	for _, r := range m.hunkRow {
		if r > m.cursor {
			m.jumpTo(r)
			return
		}
	}
}

// PrevHunk moves the cursor to the closest hunk header above it, which is the
// current hunk's own header when the cursor is inside the hunk.
func (m *Model) PrevHunk() {
	for i := len(m.hunkRow) - 1; i >= 0; i-- {
		if m.hunkRow[i] < m.cursor {
			m.jumpTo(m.hunkRow[i])
			return
		}
	}
}

// jumpTo puts the cursor on row r and the row at the top of the view.
func (m *Model) jumpTo(r int) {
	m.cursor, m.top = r, r
	m.clamp()
}

// --- horizontal scrolling ---

// ScrollRight shifts the code area n cells to the right (reveals later text).
func (m *Model) ScrollRight(n int) { m.xoff += n; m.clamp() }

// ScrollLeft shifts the code area n cells to the left.
func (m *Model) ScrollLeft(n int) { m.xoff -= n; m.clamp() }

// HalfScrollRight scrolls right by half the code area.
func (m *Model) HalfScrollRight() { m.ScrollRight(max(m.codeW()/2, 1)) }

// HalfScrollLeft scrolls left by half the code area.
func (m *Model) HalfScrollLeft() { m.ScrollLeft(max(m.codeW()/2, 1)) }

// XOffset returns the horizontal scroll in cells.
func (m *Model) XOffset() int { return m.xoff }

// Top returns the index of the first visible row (hunk headers count as rows).
func (m *Model) Top() int { return m.top }

// Cursor describes the diff line under the cursor. ok is false when there is
// no file, or the cursor is on a hunk header or a "no newline" marker.
func (m *Model) Cursor() (c CursorLine, ok bool) {
	if m.file == nil || m.cursor >= len(m.rows) {
		return CursorLine{}, false
	}
	r := m.rows[m.cursor]
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
	end := min(m.top+m.h, len(m.rows))
	lines := make([]string, 0, end-m.top)
	for i := m.top; i < end; i++ {
		lines = append(lines, m.renderRow(i))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderMessage() string {
	lines := m.messageLines()
	var out []string
	for _, l := range lines {
		out = append(out, strings.Split(ansi.Wrap(l, m.w, ""), "\n")...)
	}
	if len(out) > m.h {
		out = out[:m.h]
	}
	for i, l := range out {
		out[i] = m.th.Muted.Render(l)
	}
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
