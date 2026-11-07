package userinterface

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/matching"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// helpSearchPrefix is what the bottom line shows while the help's search is
// typed, the same as the lists' search.
const helpSearchPrefix = "/"

// helpGutter is the left column that carries the match marker, as in lists.
const helpGutter = 2

// helpSearch is the search of the open help overlay. The zero value is no
// search. The overlay has no cursor, so the search tracks the row it last
// landed on.
type helpSearch struct {
	query string
	// current is the body row the search last landed on; n and N go from it.
	current int
	// last is the last query submitted; enter on an empty prompt repeats it.
	last string
	// While typing: where the view was and the search to go back to on esc.
	originOffset    int
	previousQuery   string
	previousCurrent int
}

func (search helpSearch) active() bool { return search.query != "" }

// helpMarking reports whether matching rows carry the marker: a search is
// active and the hlsearch option is on.
func (model *Model) helpMarking() bool {
	return model.helpSearch.active() && model.options.HighlightSearch()
}

// helpSearchMatches returns the body rows that match the search, found in the
// rows as drawn without styling, so wrapped rows each match on their own.
func (model *Model) helpSearchMatches() []int {
	lines := model.helpLines()
	rows := make([]string, len(lines))
	for i, line := range lines {
		rows[i] = ansi.Strip(line)
	}
	return matching.Rows(model.helpSearch.query, rows)
}

// markedHelpLines is the body to draw, each row behind a gutter in which, with
// the marker on, matching rows carry the theme's search marker.
func (model *Model) markedHelpLines() []string {
	lines := model.helpLines()
	var matches []int
	if model.helpMarking() {
		matches = model.helpSearchMatches()
	}
	marked := make([]string, len(lines))
	for i, line := range lines {
		gutter := strings.Repeat(" ", helpGutter)
		if slices.Contains(matches, i) {
			gutter = model.theme.SearchMarker.Render("•") + " "
		}
		marked[i] = gutter + line
	}
	return marked
}

// showHelpRow scrolls the least needed to bring a body row into view.
func (model *Model) showHelpRow(row int) {
	_, height := model.contentSize()
	rows := helpBodyRows(height)
	switch {
	case row < model.helpOffset:
		model.helpOffset = row
	case row >= model.helpOffset+rows:
		model.helpOffset = row - rows + 1
	}
	model.clampHelp()
}

// beginHelpSearch opens the prompt for a new search from the top of the view.
func (model *Model) beginHelpSearch() tea.Cmd {
	search := &model.helpSearch
	search.originOffset, search.previousQuery, search.previousCurrent = model.helpOffset, search.query, search.current
	return model.commandLine.openPrompt(helpSearchPrefix)
}

// helpPrompt applies what the shell reports about the search prompt, the way
// the lists' search does: typing lands on the first match from the top of the
// view; enter keeps the search; esc goes back to the view and the search
// before; enter on an empty prompt repeats the last search.
func (model *Model) helpPrompt(result PromptResultMessage) tea.Cmd {
	if result.Prefix != helpSearchPrefix {
		return nil
	}
	search := &model.helpSearch
	query := strings.TrimSpace(result.Text)
	switch result.State {
	case PromptChanged:
		model.typeHelpSearch(query)
	case PromptCancelled:
		model.typeHelpSearch(search.previousQuery)
		search.current = search.previousCurrent
		model.helpOffset = search.originOffset
		model.clampHelp()
	case PromptSubmitted:
		if query == "" {
			query = search.last
		}
		search.last = query
		if model.typeHelpSearch(query) == 0 && query != "" {
			return Info("no match: " + terminaltext.Clean(query))
		}
	}
	return nil
}

// typeHelpSearch sets the query and lands on the first match at or after the
// top of the view, wrapping to the first match. It returns the match count.
func (model *Model) typeHelpSearch(query string) int {
	search := &model.helpSearch
	search.query = query
	search.current = search.originOffset - 1
	model.helpOffset = search.originOffset
	matches := model.helpSearchMatches()
	if len(matches) == 0 {
		model.clampHelp()
		return 0
	}
	row := matches[0]
	for _, match := range matches {
		if match >= search.originOffset {
			row = match
			break
		}
	}
	search.current = row
	model.showHelpRow(row)
	return len(matches)
}

// stepHelpSearch handles n (forward) and N: the count-th next matching row,
// wrapping when wrapscan is on, with the messages the lists' search gives.
func (model *Model) stepHelpSearch(forward bool, count int) tea.Cmd {
	search := &model.helpSearch
	if !search.active() {
		return nil
	}
	matches := model.helpSearchMatches()
	if len(matches) == 0 {
		return Info("no match: " + terminaltext.Clean(search.query))
	}
	wrap := model.options.WrapScan()
	row := search.current
	if _, height := model.contentSize(); row < model.helpOffset || row >= model.helpOffset+helpBodyRows(height) {
		// The match is off screen (the user scrolled): step from the top of
		// the view, as a pager does.
		row = model.helpOffset - 1
		if !forward {
			row = model.helpOffset
		}
	}
	for range max(count, 1) {
		next, ok := neighbouringMatch(matches, row, forward, wrap)
		if !ok {
			if forward {
				return Info("no more matches below")
			}
			return Info("no more matches above")
		}
		row = next
	}
	search.current = row
	model.showHelpRow(row)
	return nil
}

// helpSearchHint shows the active search on the title line.
func (model *Model) helpSearchHint() string {
	if !model.helpSearch.active() {
		return ""
	}
	return "  " + helpSearchPrefix + terminaltext.Clean(model.helpSearch.query)
}

// neighbouringMatch is the first match after row (or the last before it), or
// with wrap the first (last) match overall.
func neighbouringMatch(matches []int, row int, forward, wrap bool) (int, bool) {
	if forward {
		for _, match := range matches {
			if match > row {
				return match, true
			}
		}
		return matches[0], wrap
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i] < row {
			return matches[i], true
		}
	}
	return matches[len(matches)-1], wrap
}
