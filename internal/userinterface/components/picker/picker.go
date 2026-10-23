// Package picker is a small filterable list shown as a popup. The owning view
// must capture keys while the picker is open (userinterface.KeyCapturer) and feed every
// key to Update; esc closes it.
package picker

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/components/scroll"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
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
	matches []int         // indexes into items
	win     scroll.Window // cursor and top row over matches
}

// New returns a picker over items.
func New(theme theme.Theme, title string, items []string) *Model {
	model := &Model{th: theme, title: title, items: items}
	model.refilter()
	return model
}

// Query returns the current filter text.
func (model *Model) Query() string { return string(model.query) }

// Matches returns the number of items passing the filter.
func (model *Model) Matches() int { return len(model.matches) }

// Update handles one key press. Every key is consumed: printable text edits
// the filter, so letters never trigger anything outside the picker.
func (model *Model) Update(message tea.KeyPressMsg) Result {
	switch message.String() {
	case "esc":
		return Result{Outcome: Cancelled}
	case "enter":
		if len(model.matches) == 0 {
			return Result{Outcome: Open}
		}
		return Result{Outcome: Chosen, Index: model.matches[model.win.Cursor()]}
	case "up", "ctrl+p":
		model.win.Up(1)
	case "down", "ctrl+n":
		model.win.Down(1)
	case "backspace":
		if count := len(model.query); count > 0 {
			model.query = model.query[:count-1]
			model.refilter()
		}
	case "ctrl+u":
		model.query = nil
		model.refilter()
	default:
		if text := message.Text; text != "" && !strings.ContainsFunc(text, unicode.IsControl) {
			model.query = append(model.query, []rune(text)...)
			model.refilter()
		}
	}
	return Result{Outcome: Open}
}

func (model *Model) refilter() {
	model.matches = model.matches[:0]
	query := strings.ToLower(string(model.query))
	for i, item := range model.items {
		if subsequence(strings.ToLower(item), query) {
			model.matches = append(model.matches, i)
		}
	}
	model.win.Reset()
	model.win.SetRows(len(model.matches))
}

func subsequence(text, query string) bool {
	runes := []rune(text)
	i := 0
	for _, character := range query {
		for i < len(runes) && runes[i] != character {
			i++
		}
		if i == len(runes) {
			return false
		}
		i++
	}
	return true
}

// Render draws the popup, sized to fit within width x height. It returns ""
// when there is no room.
func (model *Model) Render(width, height int) string {
	paneWidth := min(64, width)
	if paneWidth < 12 || height < 5 {
		return ""
	}
	inner := paneWidth - 2
	rows := min(max(len(model.matches), 1), min(height-4, 12))

	lines := make([]string, 0, rows+2)
	query := string(model.query)
	if over := ansi.StringWidth(query) - (inner - 3); over > 0 {
		query = ansi.TruncateLeft(query, over+1, "…") // keep the tail of a long query visible
	}
	lines = append(lines, model.th.Title.Render("> ")+model.th.Normal.Render(query)+model.th.Muted.Render("▏"))
	if len(model.matches) == 0 {
		lines = append(lines, model.th.Muted.Render("no matches"))
	}
	model.win.SetSize(rows)
	for i := model.win.Top(); i < len(model.matches) && i < model.win.Top()+rows; i++ {
		label := model.items[model.matches[i]]
		if over := ansi.StringWidth(label) - (inner - 4); over > 0 {
			label = ansi.TruncateLeft(label, over+1, "…") // keep the file name visible
		}
		if i == model.win.Cursor() {
			lines = append(lines, model.th.OverlayItemFocus.Render(pad("> "+label, inner-2)))
		} else {
			lines = append(lines, model.th.OverlayItem.Render("  "+label))
		}
	}
	box := model.th.OverlayBox.
		Width(paneWidth).
		Render(strings.Join(lines, "\n"))
	return model.titled(box)
}

// titled puts the title in the top edge of the box.
func (model *Model) titled(box string) string {
	if model.title == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	text := " " + model.title + " "
	width := ansi.StringWidth(lines[0])
	if ansi.StringWidth(text)+4 > width {
		return box
	}
	lines[0] = ansi.Cut(lines[0], 0, 2) + model.th.OverlayBorder.Render(text) + ansi.Cut(lines[0], 2+ansi.StringWidth(text), width)
	return strings.Join(lines, "\n")
}

func pad(text string, width int) string {
	if count := width - ansi.StringWidth(text); count > 0 {
		return text + strings.Repeat(" ", count)
	}
	return text
}
