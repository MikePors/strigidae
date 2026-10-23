package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

const tabGap = " "

// line is one row of the scrolling area under the tab bar.
type line struct {
	section int
	// row is the index within the section's rows, or one of the kinds below.
	row int
}

const (
	headingLine = -1 // the section's title
	noteLine    = -2 // loading, empty or error text
)

// Render implements userinterface.View.
func (view *View) Render(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	view.height = height
	if len(view.tabs) == 0 {
		return clip(view.theme.Muted.Render("no tabs configured"), width, height)
	}
	current := &view.tabs[view.active]
	table := pullrequestrows.New(view.theme, view.now, true)
	layout := table.Layout(current.pullRequests(), width-pullrequestrows.MarkerColumns)

	lines, selectedLine := current.lines()
	current.window.SetRows(len(lines))
	current.window.SetSize(view.listHeight())
	if current.selectedIndex() == 0 {
		current.window.GotoRow(0) // the tab's first row: show everything above it too
	}
	if current.selectedIndex() == current.total()-1 {
		current.window.GotoRow(len(lines) - 1) // the last row: show any section after it too
	}
	if selectedLine > 0 && lines[selectedLine-1].row == headingLine {
		current.window.GotoRow(selectedLine - 1) // keep the section's heading on screen
	}
	current.window.GotoRow(selectedLine)

	selectedIndex := current.selectedIndex()
	out := []string{view.tabBar(width), table.Header(layout, width)}
	end := min(len(lines), current.window.Top()+view.listHeight())
	for i := current.window.Top(); i < end; i++ {
		entry := lines[i]
		target := current.sections[entry.section]
		switch entry.row {
		case headingLine:
			out = append(out, view.heading(target, width))
		case noteLine:
			out = append(out, view.note(target, width))
		default:
			selected := current.firstRow(entry.section)+entry.row == selectedIndex
			out = append(out, table.Row(layout, target.pullRequests[entry.row], selected, width))
		}
	}
	return clip(strings.Join(out, "\n"), width, height)
}

// lines lays out every section and reports which line holds the selection.
func (current *tab) lines() ([]line, int) {
	selectedIndex := current.selectedIndex()
	var lines []line
	selectedLine := 0
	for i, target := range current.sections {
		lines = append(lines, line{section: i, row: headingLine})
		if len(target.pullRequests) == 0 {
			lines = append(lines, line{section: i, row: noteLine})
			continue
		}
		for row := range target.pullRequests {
			if current.firstRow(i)+row == selectedIndex {
				selectedLine = len(lines)
			}
			lines = append(lines, line{section: i, row: row})
		}
	}
	return lines, selectedLine
}

// tabBar draws the tab names with the active one highlighted. When they do
// not all fit, tabs before the active one are dropped from the left.
func (view *View) tabBar(width int) string {
	chips := make([]string, len(view.tabs))
	widths := make([]int, len(view.tabs))
	for i, current := range view.tabs {
		label := " " + terminaltext.Clean(current.name) + " "
		widths[i] = ansi.StringWidth(label)
		if i == view.active {
			chips[i] = view.theme.Dashboard.ActiveTab.Render(label)
		} else {
			chips[i] = view.theme.Dashboard.InactiveTab.Render(label)
		}
	}
	first := 0
	for first < view.active && sum(widths[first:view.active+1])+len(tabGap)*(view.active-first) > width {
		first++
	}
	return ansi.Truncate(strings.Join(chips[first:], tabGap), width, "")
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func (view *View) heading(target section, width int) string {
	title := view.theme.SectionHeading.Render(terminaltext.Clean(target.name))
	if target.err != nil && len(target.pullRequests) > 0 {
		title += " " + view.theme.ErrorText.Render("(refresh failed: "+terminaltext.SafeError(target.err).Error()+")")
	}
	query := "  " + view.theme.Muted.Render(terminaltext.Clean(target.query))
	return ansi.Truncate(title+query, width, "…")
}

func (view *View) note(target section, width int) string {
	switch {
	case target.loading:
		return view.theme.Muted.Render("  Loading...")
	case target.err != nil:
		message := terminaltext.SafeError(target.err).Error()
		return view.theme.ErrorText.Render(ansi.Truncate("  Could not load: "+message, width, "…"))
	default:
		return view.theme.Muted.Render("  No pull requests")
	}
}

// clip cuts text to at most height lines of width columns.
func clip(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	lines = lines[:min(len(lines), height)]
	for i, current := range lines {
		lines[i] = ansi.Truncate(current, width, "")
	}
	return strings.Join(lines, "\n")
}
