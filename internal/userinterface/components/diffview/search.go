package diffview

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// The search is the lists' (components/search) over the pane's drawn rows: a
// diff line's code text or one drawn row of a thread. Hunk headers, notices
// and placeholders have no text to match. The query is kept while the rows
// are rebuilt and dropped when another file is shown.

// StartSearch begins typing a search from the cursor. The host opens the
// prompt when TakeSearchRequest reports true.
func (model *Model) StartSearch() tea.Cmd { return model.search.Start(model.searchList()) }

// TakeSearchRequest reports whether a prompt was asked for since the last
// call, and forgets it.
func (model *Model) TakeSearchRequest() bool { return model.search.TakeRequest() }

// SearchPrompt applies what the shell reports about the search prompt.
func (model *Model) SearchPrompt(result userinterface.PromptResultMessage) tea.Cmd {
	return model.search.Prompt(result, model.searchList())
}

// SearchStep moves to the count-th next (or previous) match, wrapping past
// the last one when wrapScan is on.
func (model *Model) SearchStep(message keys.Message, forward, wrapScan bool) tea.Cmd {
	return model.search.Step(message, forward, model.searchList(), wrapScan)
}

// ClearSearch drops the search and its marks.
func (model *Model) ClearSearch() { model.search.Clear() }

// SetHighlightSearch marks the rows that match the search, or stops marking
// them (the hlsearch option).
func (model *Model) SetHighlightSearch(on bool) { model.highlightSearch = on }

// searchMarked reports whether row i is drawn with the match marker.
func (model *Model) searchMarked(i int) bool { return model.highlightSearch && model.search.Marked(i) }

func (model *Model) searchList() search.List {
	return search.List{
		Items:  model.searchItems,
		Cursor: model.window.Cursor,
		GoTo: func(row int) tea.Cmd {
			model.window.GotoRow(row)
			return nil
		},
	}
}

// searchItems returns every row as the search sees it.
func (model *Model) searchItems() []search.Item {
	items := make([]search.Item, len(model.rows))
	// Identities must survive a gap opening, which numbers the base rows anew:
	// a diff line is its hunk and its place among the hunk's lines.
	hunk, inHunk := -1, 0
	for i, row := range model.rows {
		if row.threadRow == nil && !row.message && !row.revealed && row.line == nil {
			hunk, inHunk = row.hunk, 0
		}
		switch {
		case row.threadRow != nil:
			thread := model.fileThreads[row.threadRow.threadIndex]
			text := ansi.Strip(model.blocks[row.threadRow.threadIndex][row.threadRow.rowInBlock])
			items[i] = search.Item{Text: text, Identity: "thread:" + thread.Identifier + ":" + strconv.Itoa(row.threadRow.rowInBlock)}
		case row.revealed:
			items[i] = search.Item{Text: row.text, Identity: "shown:" + strconv.Itoa(row.line.New)}
		case row.isCode():
			items[i] = search.Item{Text: row.text, Identity: "line:" + strconv.Itoa(hunk) + ":" + strconv.Itoa(inHunk)}
			inHunk++
		case row.gap != nil:
			items[i] = search.Item{Identity: "gap:" + strconv.Itoa(row.gap.index)}
		case row.line == nil && !row.message:
			items[i] = search.Item{Identity: "hunk:" + strconv.Itoa(row.hunk)}
		default:
			items[i] = search.Item{Identity: "base:" + strconv.Itoa(row.baseIndex)}
		}
	}
	return items
}
