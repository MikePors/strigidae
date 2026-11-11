package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
)

// commandCopyURL is the :copy-url command, which the workspace has too.
const commandCopyURL = "copy-url"

// Commands lists the command names of the dashboard, for the shell to tell a
// command another view owns from a word that is no command at all.
func Commands() []string { return []string{commandCopyURL} }

// CommandNames implements userinterface.CommandLister.
func (view *View) CommandNames() []string { return Commands() }

// Command implements userinterface.Commander.
func (view *View) Command(name, _ string) (tea.Cmd, bool) {
	if name == commandCopyURL {
		return view.copyURL(), true
	}
	return nil, false
}

// copyURL copies the URL of the pull request under the cursor, in whichever
// section it is. With no row there is nothing to copy.
func (view *View) copyURL() tea.Cmd {
	pullRequest, _ := view.selected()
	return userinterface.CopyURL(view.clipboard, view.timeouts, pullRequest.URL)
}
