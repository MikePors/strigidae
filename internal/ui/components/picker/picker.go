// Package picker is a small filterable list shown as a popup. The owning view
// must capture keys while the picker is open (ui.KeyCapturer) and feed every
// key to Update; esc closes it.
package picker

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/ui/theme"
)

// Outcome is what a key press did to the picker.
type Outcome int

// Outcomes of Update.
const (
	// Open means the picker stays open.
	Open Outcome = iota
	// Chosen means the user picked an item; Result.Index says which.
	Chosen
	// Cancelled means the user pressed esc.
	Cancelled
)

// Result is returned by Update.
type Result struct {
	Outcome Outcome
	// Index is the chosen item's index in the slice given to New.
	Index int
}

// Model is a filterable list. Matching is a case-insensitive subsequence test,
// so "wsgo" finds "views/workspace/workspace.go".
type Model struct {
	th      theme.Theme
	title   string
	items   []string
	query   []rune
	matches []int // indexes into items
	cursor  int   // index into matches
}

// New returns a picker over items.
func New(th theme.Theme, title string, items []string) *Model {
	m := &Model{th: th, title: title, items: items}
	m.refilter()
	return m
}

// Query returns the current filter text.
func (m *Model) Query() string { return string(m.query) }

// Matches returns the number of items passing the filter.
func (m *Model) Matches() int { return len(m.matches) }

// Update handles one key press. Every key is consumed: printable text edits
// the filter, so letters never trigger anything outside the picker.
func (m *Model) Update(msg tea.KeyPressMsg) Result {
	switch msg.String() {
	case "esc":
		return Result{Outcome: Cancelled}
	case "enter":
		if len(m.matches) == 0 {
			return Result{Outcome: Open}
		}
		return Result{Outcome: Chosen, Index: m.matches[m.cursor]}
	case "up", "ctrl+p":
		m.cursor = max(0, m.cursor-1)
	case "down", "ctrl+n":
		m.cursor = max(0, min(len(m.matches)-1, m.cursor+1))
	case "backspace":
		if n := len(m.query); n > 0 {
			m.query = m.query[:n-1]
			m.refilter()
		}
	case "ctrl+u":
		m.query = nil
		m.refilter()
	default:
		if txt := msg.Text; txt != "" && !strings.ContainsFunc(txt, unicode.IsControl) {
			m.query = append(m.query, []rune(txt)...)
			m.refilter()
		}
	}
	return Result{Outcome: Open}
}

func (m *Model) refilter() {
	m.matches = m.matches[:0]
	q := strings.ToLower(string(m.query))
	for i, it := range m.items {
		if subsequence(strings.ToLower(it), q) {
			m.matches = append(m.matches, i)
		}
	}
	m.cursor = 0
}

func subsequence(s, q string) bool {
	rs := []rune(s)
	i := 0
	for _, r := range q {
		for i < len(rs) && rs[i] != r {
			i++
		}
		if i == len(rs) {
			return false
		}
		i++
	}
	return true
}

// Render draws the popup, sized to fit within width x height. It returns ""
// when there is no room.
func (m *Model) Render(width, height int) string {
	w := min(64, width)
	if w < 12 || height < 5 {
		return ""
	}
	inner := w - 2
	rows := min(max(len(m.matches), 1), min(height-4, 12))

	lines := make([]string, 0, rows+2)
	q := string(m.query)
	if over := ansi.StringWidth(q) - (inner - 3); over > 0 {
		q = ansi.TruncateLeft(q, over+1, "…") // keep the tail of a long query visible
	}
	lines = append(lines, m.th.Title.Render("> ")+m.th.Normal.Render(q)+m.th.Muted.Render("▏"))
	if len(m.matches) == 0 {
		lines = append(lines, m.th.Muted.Render("no matches"))
	}
	start := min(max(0, m.cursor-rows+1), max(0, len(m.matches)-rows))
	for i := start; i < len(m.matches) && i < start+rows; i++ {
		label := m.items[m.matches[i]]
		if over := ansi.StringWidth(label) - (inner - 4); over > 0 {
			label = ansi.TruncateLeft(label, over+1, "…") // keep the file name visible
		}
		if i == m.cursor {
			lines = append(lines, m.th.OverlayItemFocus.Render(pad("> "+label, inner-2)))
		} else {
			lines = append(lines, m.th.OverlayItem.Render("  "+label))
		}
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.th.OverlayBorder.GetForeground()).
		Width(w).
		Render(strings.Join(lines, "\n"))
	return m.titled(box)
}

// titled puts the title in the top edge of the box.
func (m *Model) titled(box string) string {
	if m.title == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	t := " " + m.title + " "
	w := ansi.StringWidth(lines[0])
	if ansi.StringWidth(t)+4 > w {
		return box
	}
	lines[0] = ansi.Cut(lines[0], 0, 2) + m.th.OverlayBorder.Render(t) + ansi.Cut(lines[0], 2+ansi.StringWidth(t), w)
	return strings.Join(lines, "\n")
}

func pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
