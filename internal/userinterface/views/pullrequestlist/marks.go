package pullrequestlist

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
)

// The marks commands act on the pull request under the cursor in the active
// tab. The marks themselves live in the root; the list keeps the last it was
// sent and asks the root to change them.
const (
	commandMark   = "mark"
	commandUnmark = "unmark"
)

// markOf is the mark of a pull request in this list, or "" without one.
func (view *View) markOf(pullRequest github.PullRequest) string {
	text, _ := view.marks.Mark(userinterface.MarkKey(userinterface.ClientHost(view.client), pullRequest))
	return text
}

func (view *View) mark(text string) tea.Cmd {
	pullRequest, found := view.current().selected()
	return userinterface.MarkCommand(userinterface.ClientHost(view.client), pullRequest, found, text)
}

func (view *View) unmark() tea.Cmd {
	pullRequest, found := view.current().selected()
	return userinterface.UnmarkCommand(userinterface.ClientHost(view.client), view.marks, pullRequest, found)
}

// CompleteArguments implements userinterface.ArgumentCompleter: `:mark` offers
// the marks already in use, `:sort` its keys.
func (view *View) CompleteArguments(name string) []string {
	switch name {
	case commandMark:
		return view.marks.Names()
	case commandSort:
		return pullrequestrows.SortCompletions()
	}
	return nil
}
