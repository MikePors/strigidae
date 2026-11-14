package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/pullrequestrows"
)

// commandCopyURL is the :copy-url command, which the workspace has too.
const commandCopyURL = "copy-url"

// Commands lists the command names of the dashboard, for the shell to tell a
// command another view owns from a word that is no command at all.
func Commands() []string { return []string{commandCopyURL, commandMark, commandUnmark, commandSort} }

// CommandNames implements userinterface.CommandLister.
func (view *View) CommandNames() []string { return Commands() }

// Command implements userinterface.Commander.
func (view *View) Command(name, arguments string) (tea.Cmd, bool) {
	switch name {
	case commandCopyURL:
		return view.copyURL(), true
	case commandMark:
		return view.mark(arguments), true
	case commandUnmark:
		return view.unmark(), true
	case commandSort:
		return view.sort(arguments), true
	}
	return nil, false
}

// commandSort is the :sort command, which the repository list has too.
const commandSort = "sort"

// sort reorders the rows of every section of the active tab, keeping the
// cursor on the same pull request. Nothing is fetched.
func (view *View) sort(arguments string) tea.Cmd {
	order, err := pullrequestrows.ParseSort(arguments)
	if err != nil {
		return userinterface.Error(err)
	}
	if len(view.tabs) == 0 {
		return nil
	}
	current := &view.tabs[view.active]
	current.order = order
	if current.anchor != nil {
		current.selectIndex(current.selectedIndex())
	}
	current.search.Refresh(current.searchList())
	return nil
}

// copyURL copies the URL of the pull request under the cursor, in whichever
// section it is. With no row there is nothing to copy.
func (view *View) copyURL() tea.Cmd {
	pullRequest, _ := view.selected()
	return userinterface.CopyURL(view.clipboard, view.timeouts, pullRequest.URL)
}
