package dashboard

import "github.com/MikePors/strigidae/internal/github"

// anchor is where the user put the cursor. It names the section as well as
// the pull request, because one pull request often appears in several
// sections, and it is resolved only within that section so results arriving
// in any order cannot move the cursor into another one.
type anchor struct {
	section    int
	mark       string
	repository github.Repository
	number     int
	// row is the fallback when the pull request is gone after a reload.
	row int
}

// total is the number of rows across all sections.
func (current *tab) total() int {
	count := 0
	for _, target := range current.displaySections() {
		count += len(target.pullRequests)
	}
	return count
}

// pullRequests lists every row of every section, top to bottom.
func (current *tab) pullRequests() []github.PullRequest {
	var all []github.PullRequest
	for _, target := range current.displaySections() {
		all = append(all, target.pullRequests...)
	}
	return all
}

// position is the section and row the cursor is on. Until the user moves it,
// it is the first row of the first section that has any, however the results
// arrived.
func (current *tab) position() (section, row int, ok bool) {
	if current.total() == 0 {
		return 0, 0, false
	}
	if current.anchor == nil {
		for i, target := range current.displaySections() {
			if len(target.pullRequests) > 0 {
				return i, 0, true
			}
		}
	}
	placed := current.anchor
	sections := current.displaySections()
	section = placed.section
	missing := false
	if placed.mark != "" {
		section = len(sections)
		for i, target := range sections {
			if target.mark != "" && target.mark >= placed.mark {
				section = i
				break
			}
		}
		missing = section == len(sections) || sections[section].mark != placed.mark
	}
	var rows []github.PullRequest
	if !missing && section < len(sections) {
		rows = sections[section].pullRequests
	}

	for i, pullRequest := range rows {
		if pullRequest.Repository == placed.repository && pullRequest.Number == placed.number {
			return section, i, true
		}
	}
	if len(rows) > 0 {
		return section, min(placed.row, len(rows)-1), true
	}
	// The section has no rows now: take the nearest section that does.
	next := section + 1
	if missing {
		next = section
	}
	for i := next; i < len(sections); i++ {
		if len(sections[i].pullRequests) > 0 {
			return i, 0, true
		}
	}
	for i := section - 1; i >= 0; i-- {
		if rows := sections[i].pullRequests; len(rows) > 0 {
			return i, len(rows) - 1, true
		}
	}
	return 0, 0, false
}

// selectedIndex is the cursor's index among all rows.
func (current *tab) selectedIndex() int {
	section, row, ok := current.position()
	if !ok {
		return 0
	}
	return current.firstRow(section) + row
}

func (current *tab) selectedPullRequest() (github.PullRequest, bool) {
	section, row, ok := current.position()
	if !ok {
		return github.PullRequest{}, false
	}
	return current.displaySections()[section].pullRequests[row], true
}

// selectIndex moves the cursor to a row index, clamped to the rows there are.
func (current *tab) selectIndex(index int) {
	total := current.total()
	if total == 0 {
		return
	}
	index = min(max(index, 0), total-1)
	for i, target := range current.displaySections() {
		if index < len(target.pullRequests) {
			pullRequest := target.pullRequests[index]
			current.anchor = &anchor{section: i, mark: target.mark, repository: pullRequest.Repository, number: pullRequest.Number, row: index}
			return
		}
		index -= len(target.pullRequests)
	}
}

func (current *tab) move(count int) { current.selectIndex(current.selectedIndex() + count) }

// firstRow is the row index of the first row of a section.
func (current *tab) firstRow(index int) int {
	count := 0
	for _, target := range current.displaySections()[:index] {
		count += len(target.pullRequests)
	}
	return count
}

// moveSection selects the first row of the next (or previous) section that
// has rows. At the last (or first) such section it does nothing.
func (current *tab) moveSection(step int) {
	sections := current.displaySections()
	section, _, ok := current.position()
	if !ok {
		return
	}
	for i := section + step; i >= 0 && i < len(sections); i += step {
		if len(sections[i].pullRequests) > 0 {
			current.selectIndex(current.firstRow(i))
			return
		}
	}
}
