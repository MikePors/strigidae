package pullrequestlist

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

const (
	gap          = "  "
	marker       = "> " // prefix of the selected row, so selection survives without colour
	noMarker     = "  "
	draftPrefix  = "[draft] "
	minTitleCols = 12
	maxAuthor    = 16
	ageCols      = 4
	reviewCols   = 2
	checksCols   = 2
	commentCols  = 3
)

// Render implements userinterface.View.
func (view *View) Render(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	view.width, view.height = width, height
	view.win.SetRows(len(view.pullRequests))
	view.win.SetSize(view.listHeight())

	var out string
	switch {
	case len(view.pullRequests) == 0 && view.loading:
		out = view.th.Muted.Render(fmt.Sprintf("Loading pull requests in %s...", view.repositoryName()))
	case len(view.pullRequests) == 0 && view.err != nil:
		out = view.errorState(width)
	case len(view.pullRequests) == 0:
		out = view.th.Muted.Render(fmt.Sprintf("no open pull requests in %s", view.repositoryName()))
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
	head := view.th.PullRequestList.ChangesRequested.Render("Could not load pull requests")
	body := ansi.Wrap(terminaltext.SafeError(view.err).Error(), width, "")
	return head + "\n" + body + "\n\n" + view.th.Muted.Render("Press R to retry.")
}

// layout holds the column widths for one render. A zero width hides a column.
type layout struct {
	number, title, author, age, checks, review, comments int
}

func (view *View) layout(width int) layout {
	rowLayout := layout{age: ageCols, checks: checksCols, review: reviewCols, comments: commentCols, number: len("#")}
	for _, pullRequest := range view.pullRequests {
		rowLayout.number = max(rowLayout.number, len(fmt.Sprintf("#%d", pullRequest.Number)))
		rowLayout.author = min(maxAuthor, max(rowLayout.author, ansi.StringWidth(pullRequest.Author)))
	}
	rowLayout.author = max(rowLayout.author, len("AUTHOR"))
	fits := func() bool { return rowLayout.fixed() <= width-minTitleCols }
	// Drop the least important columns first on narrow terminals.
	for _, drop := range []*int{&rowLayout.author, &rowLayout.comments, &rowLayout.checks, &rowLayout.review} {
		if fits() {
			break
		}
		*drop = 0
	}
	rowLayout.title = max(1, width-rowLayout.fixed())
	return rowLayout
}

// fixed is the width of every column but the title, including the gaps.
func (layout layout) fixed() int {
	width := layout.number + len(gap) // number, then the gap before the title
	for _, count := range []int{layout.author, layout.age, layout.checks, layout.review, layout.comments} {
		if count > 0 {
			width += count + len(gap)
		}
	}
	return width
}

func (view *View) table(width, height int) string {
	layout := view.layout(width - len(marker))
	lines := make([]string, 0, height)
	lines = append(lines, view.th.PullRequestList.Header.Render(pad(noMarker+layout.row("#", "TITLE", "AUTHOR", "UPD", "CI", "RV", "CMT"), width)))

	end := min(len(view.pullRequests), view.win.Top()+view.listHeight())
	for i := view.win.Top(); i < end; i++ {
		lines = append(lines, view.row(layout, view.pullRequests[i], i == view.win.Cursor(), width))
	}
	return strings.Join(lines, "\n")
}

// row pads each cell to its column and joins them. Right-aligned columns are
// numbers; the rest are left-aligned.
func (layout layout) row(number, title, author, age, checks, review, comments string) string {
	cells := []string{padLeft(number, layout.number), pad(title, layout.title)}
	add := func(text string, width int, right bool) {
		switch {
		case width == 0:
		case right:
			cells = append(cells, padLeft(text, width))
		default:
			cells = append(cells, pad(text, width))
		}
	}
	add(author, layout.author, false)
	add(age, layout.age, true)
	add(checks, layout.checks, false)
	add(review, layout.review, false)
	add(comments, layout.comments, true)
	return strings.Join(cells, gap)
}

func (view *View) row(layout layout, pullRequest github.PullRequest, selected bool, width int) string {
	title := terminaltext.Clean(pullRequest.Title)
	if pullRequest.Draft {
		title = draftPrefix + title
	}
	title = ansi.Truncate(title, layout.title, "…")
	author := ansi.Truncate(terminaltext.Clean(pullRequest.Author), layout.author, "…")
	number := fmt.Sprintf("#%d", pullRequest.Number)
	age := relativeAge(view.now(), pullRequest.UpdatedAt)
	checksMark, checksStyle := view.checksMark(pullRequest.Checks)
	reviewMark, reviewStyle := view.reviewMark(pullRequest.ReviewDecision)
	comments := commentText(pullRequest.CommentCount)

	if selected {
		return view.th.PullRequestList.Selected.Render(pad(marker+layout.row(number, title, author, age, checksMark, reviewMark, comments), width))
	}

	styles := view.th.PullRequestList
	styledTitle := view.th.Normal.Render(title)
	if pullRequest.Draft && strings.HasPrefix(title, draftPrefix) {
		styledTitle = styles.Draft.Render(draftPrefix) + view.th.Normal.Render(strings.TrimPrefix(title, draftPrefix))
	}
	// Style after padding so the escape codes don't skew the widths.
	out := []string{styles.Number.Render(padLeft(number, layout.number)), styledTitle + strings.Repeat(" ", max(0, layout.title-ansi.StringWidth(title)))}
	if layout.author > 0 {
		out = append(out, styles.Author.Render(pad(author, layout.author)))
	}
	out = append(out, styles.Age.Render(padLeft(age, layout.age)))
	if layout.checks > 0 {
		out = append(out, checksStyle.Render(pad(checksMark, layout.checks)))
	}
	if layout.review > 0 {
		out = append(out, reviewStyle.Render(pad(reviewMark, layout.review)))
	}
	if layout.comments > 0 {
		out = append(out, styles.Comments.Render(padLeft(comments, layout.comments)))
	}
	return noMarker + strings.Join(out, gap)
}
