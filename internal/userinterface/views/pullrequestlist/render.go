package pullrequestlist

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Render implements userinterface.View.
func (view *View) Render(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	view.width, view.height = width, height
	view.window.SetRows(len(view.pullRequests))
	view.window.SetSize(view.listHeight())

	var out string
	switch {
	case len(view.pullRequests) == 0 && view.loading:
		out = view.theme.Muted.Render(fmt.Sprintf("Loading pull requests in %s...", view.repositoryName()))
	case len(view.pullRequests) == 0 && view.err != nil:
		out = view.errorState(width)
	case len(view.pullRequests) == 0:
		out = view.theme.Muted.Render(fmt.Sprintf("no open pull requests in %s", view.repositoryName()))
	default:
		out = view.table(width, height)
	}
	return fit(out, width, height)
}

// fit clips s to at most height lines of width columns, however narrow the
// terminal: layouts have minimum widths that can exceed what we are given.
func fit(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	lines = lines[:min(len(lines), height)]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

func (view *View) errorState(width int) string {
	head := view.theme.PullRequestList.ChangesRequested.Render("Could not load pull requests")
	body := ansi.Wrap(terminaltext.SafeError(view.err).Error(), width, "")
	return head + "\n" + body + "\n\n" + view.theme.Muted.Render("Press R to retry.")
}

func (view *View) table(width, height int) string {
	table := pullrequestrows.New(view.theme, view.now, false)
	layout := table.Layout(view.pullRequests, width-pullrequestrows.MarkerColumns)
	lines := make([]string, 0, height)
	lines = append(lines, table.Header(layout, width))

	end := min(len(view.pullRequests), view.window.Top()+view.listHeight())
	for i := view.window.Top(); i < end; i++ {
		lines = append(lines, table.Row(layout, view.pullRequests[i], i == view.window.Cursor(), view.search.Marked(i), width))
	}
	return strings.Join(lines, "\n")
}
