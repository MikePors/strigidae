package diffview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// classNoNewline styles the text of "\ No newline" marker rows.
const classNoNewline = Class(numberClasses)

// Line-kind indexes into the style set.
const (
	kindContext = iota
	kindAdd
	kindDelete
	numberKinds
)

// variant is the set of styles for one (kind, cursor) combination.
type variant struct {
	code   [numberClasses + 1]lipgloss.Style
	fill   lipgloss.Style // pads the code area to full width
	gutter lipgloss.Style
	thread [2]lipgloss.Style // gutter marker: [0] quiet thread, [1] open thread
}

// styleSet precomputes every style combination so rendering never builds
// styles.
type styleSet struct {
	v    [2][numberKinds]variant // [cursor][kind]
	hunk [2]lipgloss.Style
}

func newStyleSet(th theme.Theme) styleSet {
	var s styleSet
	cursorBG := th.DiffCursor.GetBackground()
	bases := [numberKinds]lipgloss.Style{th.DiffContext, th.DiffAdd, th.DiffDelete}
	gutters := [numberKinds]lipgloss.Style{th.DiffGutter, th.DiffGutterAdd, th.DiffGutterDelete}
	var fg [numberClasses + 1]lipgloss.Style
	for c := range Class(numberClasses) {
		fg[c] = th.Syntax.For(c)
	}
	fg[ClassPlain] = lipgloss.NewStyle()
	fg[classNoNewline] = th.DiffNoNewline
	for cur := range 2 {
		for k := range numberKinds {
			base, gut := bases[k], gutters[k]
			if cur == 1 {
				base, gut = base.Background(cursorBG), gut.Background(cursorBG)
			}
			v := variant{fill: base, gutter: gut}
			v.thread[0] = th.Muted.Background(gut.GetBackground())
			v.thread[1] = th.ThreadMarker.Background(gut.GetBackground())
			for c := range v.code {
				v.code[c] = fg[c].Background(base.GetBackground())
			}
			s.v[cur][k] = v
		}
	}
	s.hunk[0] = th.DiffHunk
	s.hunk[1] = th.DiffHunkCursor
	return s
}

func (m *Model) renderRow(i int) string {
	r := m.rows[i]
	cur := 0
	if i == m.win.Cursor() {
		cur = 1
	}
	if r.tr != nil {
		return m.renderThreadRow(i, r)
	}
	if r.message {
		return m.renderNoticeRow(i, r)
	}
	if r.line == nil {
		return m.renderHunk(m.file.Hunks[r.hunk].Header(), cur)
	}
	var kind int
	switch r.line.Kind {
	case diff.Add:
		kind = kindAdd
	case diff.Delete:
		kind = kindDelete
	}
	mark := " "
	switch {
	case cur == 1:
		mark = ">"
	case m.selected(r.bi):
		mark, cur = markSelected, 1 // the selection takes the cursor tint
	}
	v := &m.styles.v[cur][kind]

	gutter := m.gutterText(r.line, r.bi, mark, v)
	codeW := m.codeWidth()
	if codeW == 0 {
		return ansi.Truncate(gutter, m.w, "")
	}

	spans := m.spansFor(i)
	if r.line.Kind == diff.NoNewline {
		spans = []Span{{Text: r.text, Class: classNoNewline}}
	}
	visible, used := window(spans, m.win.HorizontalOffset(), codeW)

	var b strings.Builder
	b.WriteString(gutter)
	for _, sp := range visible {
		b.WriteString(v.code[sp.Class].Render(sp.Text))
	}
	if pad := codeW - used; pad > 0 {
		b.WriteString(v.fill.Render(spaces(pad)))
	}
	return b.String()
}

// gutterText is the styled fixed part of a diff row: cursor mark, line
// numbers, thread marker, change marker.
func (m *Model) gutterText(l *diff.Line, bi int, mark string, v *variant) string {
	marker := " "
	switch l.Kind {
	case diff.Add:
		marker = "+"
	case diff.Delete:
		marker = "-"
	case diff.NoNewline:
		marker = `\`
	}
	glyph, open := m.threadGlyph(bi)
	ts := v.gutter
	if glyph != " " {
		ts = v.thread[0]
		if open {
			ts = v.thread[1]
		}
	}
	return v.gutter.Render(mark+numberCell(l.Old, m.numberWidth)+" "+numberCell(l.New, m.numberWidth)+" ") +
		ts.Render(glyph) + v.gutter.Render(marker)
}

func numberCell(n, w int) string {
	s := ""
	if n > 0 {
		s = strconv.Itoa(n)
	}
	return spaces(w-len(s)) + s
}

func (m *Model) renderHunk(header string, cur int) string {
	mark := " "
	if cur == 1 {
		mark = ">"
	}
	header, _ = displayText(header) // the section text is copied from file content
	s := ansi.Truncate(mark+header, m.w, "")
	s += spaces(m.w - ansi.StringWidth(s))
	return m.styles.hunk[cur].Render(s)
}

// spansFor returns the highlighted spans of row i, highlighting on first use.
func (m *Model) spansFor(i int) []Span {
	bi := m.rows[i].bi
	if s, ok := m.cache[bi]; ok {
		return s
	}
	s := m.highlight(m.file.Path(), m.rows[i].text)
	m.cache[bi] = s
	return s
}
