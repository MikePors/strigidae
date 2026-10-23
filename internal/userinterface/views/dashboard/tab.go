package dashboard

import "github.com/MikePors/strigidae/internal/github"

// total is the number of rows across all sections.
func (current *tab) total() int {
	count := 0
	for _, target := range current.sections {
		count += len(target.pullRequests)
	}
	return count
}

// pullRequests lists every row of every section, top to bottom.
func (current *tab) pullRequests() []github.PullRequest {
	var all []github.PullRequest
	for _, target := range current.sections {
		all = append(all, target.pullRequests...)
	}
	return all
}

func (current *tab) selectedPullRequest() (github.PullRequest, bool) {
	all := current.pullRequests()
	if current.selected < 0 || current.selected >= len(all) {
		return github.PullRequest{}, false
	}
	return all[current.selected], true
}

// restore keeps the selection on the same pull request after a reload, if it
// is still listed; otherwise the selection stays on the same row index.
func (current *tab) restore(previous github.PullRequest, had bool) {
	if had {
		for i, pullRequest := range current.pullRequests() {
			if pullRequest.Repository == previous.Repository && pullRequest.Number == previous.Number {
				current.selected = i
				return
			}
		}
	}
	current.clampSelection()
}

func (current *tab) clampSelection() {
	current.selected = min(max(current.selected, 0), max(current.total()-1, 0))
}

func (current *tab) move(count int) {
	current.selected += count
	current.clampSelection()
}

// sectionOfSelection is the index of the section holding the selected row.
func (current *tab) sectionOfSelection() int {
	remaining := current.selected
	for i, target := range current.sections {
		if remaining < len(target.pullRequests) {
			return i
		}
		remaining -= len(target.pullRequests)
	}
	return 0
}

// firstRow is the selection index of the first row of a section.
func (current *tab) firstRow(index int) int {
	count := 0
	for _, target := range current.sections[:index] {
		count += len(target.pullRequests)
	}
	return count
}

// moveSection selects the first row of the next (or previous) section that
// has rows. At the last (or first) such section it does nothing.
func (current *tab) moveSection(step int) {
	if current.total() == 0 {
		return
	}
	for i := current.sectionOfSelection() + step; i >= 0 && i < len(current.sections); i += step {
		if len(current.sections[i].pullRequests) > 0 {
			current.selected = current.firstRow(i)
			return
		}
	}
}
