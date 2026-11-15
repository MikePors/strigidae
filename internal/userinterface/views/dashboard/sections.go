package dashboard

import (
	"slices"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
)

// displaySections derives the visible rows without changing the loaded
// results: each section in the tab's sort, then the marked pull requests moved
// into a sublist per mark.
func (current *tab) displaySections() []section {
	if current.marks.Empty() {
		return current.sortedSections()
	}
	sections := make([]section, 0, len(current.sections))
	marked := make(map[string][]github.PullRequest)
	statuses := make(map[string]section)
	type identity struct {
		repository github.Repository
		number     int
	}
	seen := make(map[identity]bool)
	for _, source := range current.sortedSections() {
		target := source
		target.pullRequests = nil
		for _, pullRequest := range source.pullRequests {
			name, _ := current.marks.Mark(userinterface.MarkKey(current.host, pullRequest))
			if name == "" {
				target.pullRequests = append(target.pullRequests, pullRequest)
				continue
			}
			status := statuses[name]
			status.stale = status.stale || source.stale
			if source.err != nil {
				status.err = source.err
			}
			statuses[name] = status
			target.grouped = true
			key := identity{pullRequest.Repository, pullRequest.Number}
			if !seen[key] {
				marked[name] = append(marked[name], pullRequest)
				seen[key] = true
			}
		}
		sections = append(sections, target)
	}
	names := make([]string, 0, len(marked))
	for name := range marked {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		rows := marked[name]
		if current.order.Key == pullrequestrows.SortDefault {
			// Rows of a sublist come from several searches, so it has no
			// order of GitHub's to keep: most recently updated first.
			slices.SortStableFunc(rows, func(left, right github.PullRequest) int {
				return right.UpdatedAt.Compare(left.UpdatedAt)
			})
		} else {
			rows = current.order.Apply(rows)
		}
		target := statuses[name]
		target.name = name
		target.mark = name
		target.pullRequests = rows
		sections = append(sections, target)
	}
	return sections
}

// sortedSections is the sections with each one's rows in the tab's sort.
func (current *tab) sortedSections() []section {
	if current.order.Key == pullrequestrows.SortDefault {
		return current.sections
	}
	sections := slices.Clone(current.sections)
	for i := range sections {
		sections[i].pullRequests = current.order.Apply(sections[i].pullRequests)
	}
	return sections
}
