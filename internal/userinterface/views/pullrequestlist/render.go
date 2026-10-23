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
func (v *View) Render(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	v.width, v.height = width, height
	v.win.SetRows(len(v.pullRequests))
	v.win.SetSize(v.listHeight())

	var out string
	switch {
	case len(v.pullRequests) == 0 && v.loading:
		out = v.th.Muted.Render(fmt.Sprintf("Loading pull requests in %s...", v.repositoryName()))
	case len(v.pullRequests) == 0 && v.err != nil:
		out = v.errorState(width)
	case len(v.pullRequests) == 0:
		out = v.th.Muted.Render(fmt.Sprintf("no open pull requests in %s", v.repositoryName()))
	default:
		out = v.table(width, height)
	}
	return fit(out, width, height)
}

// fit clips s to at most height lines of width columns, however narrow the
// terminal: layouts have minimum widths that can exceed what we are given.
func fit(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	lines = lines[:min(len(lines), height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "")
	}
	return strings.Join(lines, "\n")
}

func (v *View) errorState(width int) string {
	head := v.th.PullRequestList.ChangesRequested.Render("Could not load pull requests")
	body := ansi.Wrap(terminaltext.SafeError(v.err).Error(), width, "")
	return head + "\n" + body + "\n\n" + v.th.Muted.Render("Press R to retry.")
}

// layout holds the column widths for one render. A zero width hides a column.
type layout struct {
	number, title, author, age, checks, review, comments int
}

func (v *View) layout(width int) layout {
	l := layout{age: ageCols, checks: checksCols, review: reviewCols, comments: commentCols, number: len("#")}
	for _, pr := range v.pullRequests {
		l.number = max(l.number, len(fmt.Sprintf("#%d", pr.Number)))
		l.author = min(maxAuthor, max(l.author, ansi.StringWidth(pr.Author)))
	}
	l.author = max(l.author, len("AUTHOR"))
	fit := func() bool { return l.fixed() <= width-minTitleCols }
	// Drop the least important columns first on narrow terminals.
	for _, drop := range []*int{&l.author, &l.comments, &l.checks, &l.review} {
		if fit() {
			break
		}
		*drop = 0
	}
	l.title = max(1, width-l.fixed())
	return l
}

// fixed is the width of every column but the title, including the gaps.
func (l layout) fixed() int {
	w := l.number + len(gap) // number, then the gap before the title
	for _, c := range []int{l.author, l.age, l.checks, l.review, l.comments} {
		if c > 0 {
			w += c + len(gap)
		}
	}
	return w
}

func (v *View) table(width, height int) string {
	l := v.layout(width - len(marker))
	lines := make([]string, 0, height)
	lines = append(lines, v.th.PullRequestList.Header.Render(pad(noMarker+l.row("#", "TITLE", "AUTHOR", "UPD", "CI", "RV", "CMT"), width)))

	end := min(len(v.pullRequests), v.win.Top()+v.listHeight())
	for i := v.win.Top(); i < end; i++ {
		lines = append(lines, v.row(l, v.pullRequests[i], i == v.win.Cursor(), width))
	}
	return strings.Join(lines, "\n")
}

// row pads each cell to its column and joins them. Right-aligned columns are
// numbers; the rest are left-aligned.
func (l layout) row(num, title, author, age, checks, review, comments string) string {
	cells := []string{padLeft(num, l.number), pad(title, l.title)}
	add := func(s string, w int, right bool) {
		switch {
		case w == 0:
		case right:
			cells = append(cells, padLeft(s, w))
		default:
			cells = append(cells, pad(s, w))
		}
	}
	add(author, l.author, false)
	add(age, l.age, true)
	add(checks, l.checks, false)
	add(review, l.review, false)
	add(comments, l.comments, true)
	return strings.Join(cells, gap)
}

func (v *View) row(l layout, pr github.PullRequest, selected bool, width int) string {
	title := terminaltext.Clean(pr.Title)
	if pr.Draft {
		title = draftPrefix + title
	}
	title = ansi.Truncate(title, l.title, "…")
	author := ansi.Truncate(terminaltext.Clean(pr.Author), l.author, "…")
	num := fmt.Sprintf("#%d", pr.Number)
	age := relativeAge(v.now(), pr.UpdatedAt)
	ck, ckStyle := v.checksMark(pr.Checks)
	rv, rvStyle := v.reviewMark(pr.ReviewDecision)
	cmt := commentText(pr.CommentCount)

	if selected {
		return v.th.PullRequestList.Selected.Render(pad(marker+l.row(num, title, author, age, ck, rv, cmt), width))
	}

	st := v.th.PullRequestList
	styledTitle := v.th.Normal.Render(title)
	if pr.Draft && strings.HasPrefix(title, draftPrefix) {
		styledTitle = st.Draft.Render(draftPrefix) + v.th.Normal.Render(strings.TrimPrefix(title, draftPrefix))
	}
	// Style after padding so the escape codes don't skew the widths.
	out := []string{st.Number.Render(padLeft(num, l.number)), styledTitle + strings.Repeat(" ", max(0, l.title-ansi.StringWidth(title)))}
	if l.author > 0 {
		out = append(out, st.Author.Render(pad(author, l.author)))
	}
	out = append(out, st.Age.Render(padLeft(age, l.age)))
	if l.checks > 0 {
		out = append(out, ckStyle.Render(pad(ck, l.checks)))
	}
	if l.review > 0 {
		out = append(out, rvStyle.Render(pad(rv, l.review)))
	}
	if l.comments > 0 {
		out = append(out, st.Comments.Render(padLeft(cmt, l.comments)))
	}
	return noMarker + strings.Join(out, gap)
}
