package dashboard

type lineKind int

const (
	headingLine lineKind = iota // the section's title
	noteLine                    // loading, empty or error text
	rowLine                     // one pull request
)

// line is one row of the scrolling area under the tab bar.
type line struct {
	kind    lineKind
	section int
	// row is the index within the section's rows, and index the index among
	// all rows of the tab. Both are used only by rowLine.
	row, index int
}

// lines lays out every section and reports which line holds the row at
// selectedIndex.
func (current *tab) lines(selectedIndex int) ([]line, int) {
	var lines []line
	selectedLine := 0
	index := 0
	for i, target := range current.sections {
		lines = append(lines, line{kind: headingLine, section: i})
		if len(target.pullRequests) == 0 {
			lines = append(lines, line{kind: noteLine, section: i})
			continue
		}
		for row := range target.pullRequests {
			if index == selectedIndex {
				selectedLine = len(lines)
			}
			lines = append(lines, line{kind: rowLine, section: i, row: row, index: index})
			index++
		}
	}
	return lines, selectedLine
}

// follow sizes the tab's window to lines and scrolls it to the selection,
// where height is the number of lines that fit. The rules run in this order,
// each one only moving the window further than the next would on its own:
//   - On the tab's first row, scroll to the very top, so the tab's first
//     heading and any empty section above the row stay visible.
//   - On the tab's last row, scroll to the very bottom, so an empty, loading
//     or failed section after it stays visible.
//   - On the first row of a section, first scroll to the line above it, so
//     the section's heading stays visible.
//   - Finally, scroll the minimum needed to show the selected line.
func (current *tab) follow(lines []line, selectedIndex, selectedLine, height int) {
	current.window.SetRows(len(lines))
	current.window.SetSize(height)
	if selectedIndex == 0 {
		current.window.GotoRow(0)
	}
	if selectedIndex == current.total()-1 {
		current.window.GotoRow(len(lines) - 1)
	}
	if selectedLine > 0 && lines[selectedLine-1].kind == headingLine {
		current.window.GotoRow(selectedLine - 1)
	}
	current.window.GotoRow(selectedLine)
}
