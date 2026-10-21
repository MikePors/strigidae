package diffview

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// classNoNewline styles the text of "\ No newline" marker rows.
const classNoNewline = numClasses

// Line-kind indexes into the style set.
const (
	kindContext = iota
	kindAdd
	kindDelete
	numKinds
)

// variant is the set of styles for one (kind, cursor) combination.
type variant struct {
	code   [numClasses + 1]lipgloss.Style
	fill   lipgloss.Style // pads the code area to full width
	gutter lipgloss.Style
}

// styleSet precomputes every style combination so rendering never builds
// styles.
type styleSet struct {
	v    [2][numKinds]variant // [cursor][kind]
	hunk [2]lipgloss.Style
}

func newStyleSet(th theme.Theme) styleSet {
	var s styleSet
	cursorBG := th.DiffCursor.GetBackground()
	bases := [numKinds]lipgloss.Style{th.DiffContext, th.DiffAdd, th.DiffDelete}
	gutters := [numKinds]lipgloss.Style{th.DiffGutter, th.DiffGutterAdd, th.DiffGutterDelete}
	syn := th.Syntax
	fg := [numClasses + 1]lipgloss.Style{
		ClassPlain: lipgloss.NewStyle(), ClassKeyword: syn.Keyword, ClassString: syn.String,
		ClassComment: syn.Comment, ClassNumber: syn.Number, ClassFunction: syn.Function,
		ClassType: syn.Type, ClassOperator: syn.Operator, classNoNewline: th.DiffNoNewline,
	}
	for cur := range 2 {
		for k := range numKinds {
			base, gut := bases[k], gutters[k]
			if cur == 1 {
				base, gut = base.Background(cursorBG), gut.Background(cursorBG)
			}
			v := variant{fill: base, gutter: gut}
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
	v := &m.styles.v[cur][kind]

	gutter := m.gutterText(r.line, cur == 1)
	codeW := m.codeW()
	if codeW == 0 {
		return ansi.Truncate(v.gutter.Render(gutter), m.w, "")
	}

	spans := m.spansFor(i)
	if r.line.Kind == diff.NoNewline {
		spans = []Span{{Text: r.text, Class: classNoNewline}}
	}
	visible, used := window(spans, m.win.XOffset(), codeW)

	var b strings.Builder
	b.WriteString(v.gutter.Render(gutter))
	for _, sp := range visible {
		b.WriteString(v.code[sp.Class].Render(sp.Text))
	}
	if pad := codeW - used; pad > 0 {
		b.WriteString(v.fill.Render(spaces(pad)))
	}
	return b.String()
}

func (m *Model) gutterText(l *diff.Line, cursor bool) string {
	mark := " "
	if cursor {
		mark = ">"
	}
	marker := " "
	switch l.Kind {
	case diff.Add:
		marker = "+"
	case diff.Delete:
		marker = "-"
	case diff.NoNewline:
		marker = `\`
	}
	return mark + numCell(l.Old, m.numW) + " " + numCell(l.New, m.numW) + " " + marker
}

func numCell(n, w int) string {
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
	if s, ok := m.cache[i]; ok {
		return s
	}
	s := m.highlight(m.file.Path(), m.rows[i].text)
	m.cache[i] = s
	return s
}
