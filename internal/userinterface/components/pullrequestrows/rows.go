package pullrequestrows

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

const (
	gap                 = "  "
	marker              = "> " // prefix of the selected row, so selection survives without colour
	noMarker            = "  "
	draftPrefix         = "[draft] "
	minimumTitleColumns = 12
	maximumAuthor       = 16
	maximumMark         = 16
	maximumRepository   = 24
	maximumHead         = 24
	maximumBase         = 20
	minimumRepository   = 10 // narrowest the repository column shrinks to before it is dropped
	ageColumns          = 4
	reviewColumns       = 2
	checksColumns       = 2
	commentColumns      = 3
)

// MarkerColumns is the width of the selection marker in front of every row.
// Callers subtract it from the width they give Layout.
const MarkerColumns = len(marker)

// Table draws rows with one theme and clock.
type Table struct {
	theme   theme.Theme
	now     func() time.Time
	options Options
}

// Options chooses the optional columns of a Table.
type Options struct {
	// Repository adds a column with owner/name, for lists that span
	// repositories.
	Repository bool
	// Branches adds the HEAD and BASE columns after the title.
	Branches bool
	// Mark gives the mark of a pull request, or "" when it has none. The MARK
	// column after the title is shown only while some row has a mark.
	Mark func(github.PullRequest) string
}

// New returns a Table drawing the columns options asks for.
func New(activeTheme theme.Theme, now func() time.Time, options Options) Table {
	return Table{theme: activeTheme, now: now, options: options}
}

// Layout holds the column widths for one render. A zero width hides a column.
type Layout struct {
	repository, number, title, mark, head, base, author, age, checks, review, comments int
}

// Layout sizes the columns for pullRequests in width columns (not counting the
// selection marker).
func (table Table) Layout(pullRequests []github.PullRequest, width int) Layout {
	layout := Layout{age: ageColumns, checks: checksColumns, review: reviewColumns, comments: commentColumns, number: len("#")}
	for _, pullRequest := range pullRequests {
		layout.number = max(layout.number, len(fmt.Sprintf("#%d", pullRequest.Number)))
		if mark := table.mark(pullRequest); mark != "" {
			layout.mark = min(maximumMark, max(layout.mark, ansi.StringWidth(mark)))
		}
		layout.author = min(maximumAuthor, max(layout.author, ansi.StringWidth(terminaltext.Clean(pullRequest.Author))))
		if table.options.Branches {
			layout.head = min(maximumHead, max(layout.head, ansi.StringWidth(terminaltext.Clean(pullRequest.HeadBranch))))
			layout.base = min(maximumBase, max(layout.base, ansi.StringWidth(terminaltext.Clean(pullRequest.BaseBranch))))
		}
		if table.options.Repository {
			layout.repository = min(maximumRepository, max(layout.repository, ansi.StringWidth(terminaltext.Clean(pullRequest.Repository.String()))))
		}
	}
	layout.author = max(layout.author, len("AUTHOR"))
	if layout.mark > 0 {
		layout.mark = max(layout.mark, len("MARK"))
	}
	if table.options.Branches {
		layout.head = max(layout.head, len("HEAD"))
		layout.base = max(layout.base, len("BASE"))
	}
	if table.options.Repository {
		layout.repository = max(layout.repository, len("REPO"))
	}
	fits := func() bool { return layout.fixed() <= width-minimumTitleColumns }
	// Drop the least important columns first on narrow terminals: the mark,
	// which is only a private note, goes before everything else.
	for _, drop := range []*int{&layout.mark, &layout.head, &layout.base, &layout.author, &layout.comments, &layout.checks, &layout.review} {
		if fits() {
			break
		}
		*drop = 0
	}
	if table.options.Repository {
		table.narrowRepository(&layout, width, fits)
	}
	layout.title = max(1, width-layout.fixed())
	return layout
}

// narrowRepository makes room for the title in a list that spans repositories:
// shrink the repository column to a floor, then drop it, then drop the age.
// The title takes whatever is left.
func (table Table) narrowRepository(layout *Layout, width int, fits func() bool) {
	if fits() {
		return
	}
	excess := layout.fixed() - (width - minimumTitleColumns)
	layout.repository = max(layout.repository-excess, min(layout.repository, minimumRepository))
	if fits() {
		return
	}
	layout.repository = 0
	if !fits() {
		layout.age = 0
	}
}

// mark is the clean mark of a pull request, or "" without one.
func (table Table) mark(pullRequest github.PullRequest) string {
	if table.options.Mark == nil {
		return ""
	}
	return terminaltext.Clean(table.options.Mark(pullRequest))
}

// shortRepository fits owner/name into width columns, giving up the owner
// before the name: "…/widgets" rather than "acme/wi…". text must be clean.
func shortRepository(text string, width int) string {
	if ansi.StringWidth(text) <= width {
		return text
	}
	const ownerGap = "…/" // stands in for the owner
	if slash := strings.LastIndex(text, "/"); slash >= 0 && width > ansi.StringWidth(ownerGap) {
		// Keep the name and give up the owner, cutting the name only when it
		// alone does not fit.
		return ownerGap + ansi.Truncate(text[slash+1:], width-ansi.StringWidth(ownerGap), "…")
	}
	return ansi.Truncate(text, width, "…")
}

// fixed is the width of every column but the title, including the gaps.
func (layout Layout) fixed() int {
	width := layout.number + len(gap) // number, then the gap before the title
	for _, count := range []int{layout.repository, layout.mark, layout.head, layout.base, layout.author, layout.age, layout.checks, layout.review, layout.comments} {
		if count > 0 {
			width += count + len(gap)
		}
	}
	return width
}

// Header returns the column headings, padded to width.
func (table Table) Header(layout Layout, width int) string {
	headings := rowCells{
		repository: headingCell("REPO"), number: headingCell("#"), title: headingCell("TITLE"), mark: headingCell("MARK"),
		head: headingCell("HEAD"), base: headingCell("BASE"), author: headingCell("AUTHOR"), age: headingCell("UPD"),
		checks: headingCell("CI"), review: headingCell("RV"), comments: headingCell("CMT"),
	}
	line := append(cell{{text: noMarker}}, layout.row(headings)...)
	return line.padRight(width).paintAll(table.theme.PullRequestList.Header)
}

// headingCell is a header cell; the header is painted in one style.
func headingCell(text string) cell { return styled(text, lipgloss.Style{}) }

// rowCells holds the content of every column of one row. Columns the layout
// hides are ignored.
type rowCells struct {
	repository, number, title, mark, head, base, author, age, checks, review, comments cell
}

// row pads each cell to its column and joins them with the gaps. Right-aligned
// columns are numbers; the rest are left-aligned.
func (layout Layout) row(cells rowCells) cell {
	var columns []cell
	if layout.repository > 0 {
		columns = append(columns, cells.repository.padRight(layout.repository))
	}
	columns = append(columns, cells.number.padLeft(layout.number), cells.title.padRight(layout.title))
	add := func(content cell, width int, right bool) {
		switch {
		case width == 0:
		case right:
			columns = append(columns, content.padLeft(width))
		default:
			columns = append(columns, content.padRight(width))
		}
	}
	add(cells.mark, layout.mark, false)
	add(cells.head, layout.head, false)
	add(cells.base, layout.base, false)
	add(cells.author, layout.author, false)
	add(cells.age, layout.age, true)
	add(cells.checks, layout.checks, false)
	add(cells.review, layout.review, false)
	add(cells.comments, layout.comments, true)

	var out cell
	for i, column := range columns {
		if i > 0 {
			out = append(out, segment{text: gap})
		}
		out = append(out, column...)
	}
	return out
}

// SearchText is the clean text a search looks in for a pull request: title,
// author, owner/name, head and base branch, and #number.
func SearchText(pullRequest github.PullRequest) string {
	return strings.Join([]string{
		terminaltext.Clean(pullRequest.Title),
		terminaltext.Clean(pullRequest.Author),
		terminaltext.Clean(pullRequest.Repository.String()),
		terminaltext.Clean(pullRequest.HeadBranch),
		terminaltext.Clean(pullRequest.BaseBranch),
		fmt.Sprintf("#%d", pullRequest.Number),
	}, " ")
}

// SearchItems turns pull requests into search rows: their SearchText, and the
// identity the caller gives each one.
func SearchItems(pullRequests []github.PullRequest, identity func(index int, pullRequest github.PullRequest) string) []search.Item {
	items := make([]search.Item, len(pullRequests))
	for i, pullRequest := range pullRequests {
		items[i] = search.Item{Text: SearchText(pullRequest), Identity: identity(i, pullRequest)}
	}
	return items
}

// indentColumns is how far the title moves in for each level of depth.
const indentColumns = 2

// Row draws one pull request, padded to width. The selected row is
// highlighted and carries a marker; a row that matches the search carries the
// search marker unless it is selected.
func (table Table) Row(layout Layout, pullRequest github.PullRequest, selected, matched bool, width int) string {
	return table.IndentedRow(layout, pullRequest, 0, selected, matched, width)
}

// IndentedRow is Row with the title moved in by depth levels of indentColumns
// spaces before it is truncated; the other columns stay where they are.
func (table Table) IndentedRow(layout Layout, pullRequest github.PullRequest, depth int, selected, matched bool, width int) string {
	styles := table.theme.PullRequestList
	checksMark, checksStyle := table.checksMark(pullRequest.Checks)
	reviewMark, reviewStyle := table.reviewMark(pullRequest.ReviewDecision)
	row := layout.row(rowCells{
		repository: styled(shortRepository(terminaltext.Clean(pullRequest.Repository.String()), layout.repository), styles.Repository),
		number:     styled(fmt.Sprintf("#%d", pullRequest.Number), styles.Number),
		title:      table.titleCell(pullRequest, depth, layout.title),
		mark:       styled(table.mark(pullRequest), styles.Mark).truncate(layout.mark),
		head:       styled(terminaltext.Clean(pullRequest.HeadBranch), styles.Branch).truncate(layout.head),
		base:       styled(terminaltext.Clean(pullRequest.BaseBranch), styles.Branch).truncate(layout.base),
		author:     styled(terminaltext.Clean(pullRequest.Author), styles.Author).truncate(layout.author),
		age:        styled(relativeAge(table.now(), pullRequest.UpdatedAt), styles.Age),
		checks:     styled(checksMark, checksStyle),
		review:     styled(reviewMark, reviewStyle),
		comments:   styled(commentText(pullRequest.CommentCount), styles.Comments),
	})

	if selected {
		// A selected row that matches the search shows the match glyph next to
		// the cursor, in the row's own style, which is readable on its background.
		lead := marker
		if matched {
			lead = search.SelectedGlyph
		}
		line := append(cell{{text: lead}}, row...)
		return line.padRight(width).paintAll(styles.Selected)
	}
	lead := noMarker
	if matched {
		lead = table.theme.SearchMarker.Render(search.Glyph) + " "
	}
	return lead + row.paint()
}

// titleCell is the title indented by depth levels of indentColumns spaces and
// cut to width, with a draft's marker in its own style. The indent and the
// padding after the title take the Normal style, which adds nothing today.
func (table Table) titleCell(pullRequest github.PullRequest, depth, width int) cell {
	normal := table.theme.Normal
	title := terminaltext.Clean(pullRequest.Title)
	indent := strings.Repeat(" ", indentColumns*max(depth, 0))
	if !pullRequest.Draft {
		return styled(indent+title, normal).truncate(width)
	}
	var content cell
	if indent != "" {
		content = styled(indent, normal)
	}
	content = append(content, segment{text: draftPrefix, style: table.theme.PullRequestList.Draft})
	content = append(content, segment{text: title, style: normal}).truncate(width)
	if text := content.plain(); !strings.HasPrefix(text, indent+draftPrefix) {
		// A marker cut short is no longer a marker: the whole title is plain.
		return styled(text, normal)
	}
	return content
}
