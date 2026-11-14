package pullrequestlist

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
)

// commandSort is the :sort command, which the dashboard has too.
const commandSort = "sort"

// sort reorders the active tab's loaded rows, keeping the cursor on the same
// pull request. Nothing is fetched.
func (view *View) sort(arguments string) tea.Cmd {
	order, err := pullrequestrows.ParseSort(arguments)
	if err != nil {
		return userinterface.Error(err)
	}
	current := view.current()
	current.order = order
	current.setLoaded(current.loaded)
	current.search.Refresh(view.searchListOf(current))
	return nil
}

// sortNote is what the tab bar says about the active tab's sort: its label,
// and how many rows it covers while more pages could be loaded.
func (current *tab) sortNote() string {
	label := current.order.Label()
	if label == "" || current.next == "" {
		return label
	}
	return fmt.Sprintf("%s (%d loaded)", label, len(current.loaded))
}
