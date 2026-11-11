package pullrequestlist

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
)

// commandCopyURL is the :copy-url command, which the workspace has too.
const commandCopyURL = "copy-url"

// copyURL copies the URL of the pull request under the cursor in the active
// tab. With no row there is nothing to copy.
func (view *View) copyURL() tea.Cmd {
	pullRequest, _ := view.current().selected()
	return userinterface.CopyURL(view.clipboard, view.timeouts, pullRequest.URL)
}
