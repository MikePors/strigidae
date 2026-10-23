package workspace

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// copyFilename copies the selected file's path to the clipboard. The Files
// pane cursor and the diff pane's file are the same selection, so focus does
// not matter.
func (view *View) copyFilename() tea.Cmd {
	path := view.selectedPath()
	if path == "" {
		return userinterface.Error(errors.New("no file to copy"))
	}
	// Refuse control and bidi characters so the path cannot carry escapes or
	// line breaks (a pasted newline would submit a line). It is the raw path,
	// so it may still contain shell metacharacters.
	if terminaltext.Clean(path) != path {
		return userinterface.Error(fmt.Errorf("not copied: %s contains control characters", terminaltext.Clean(path)))
	}
	return tea.Batch(tea.SetClipboard(path), userinterface.Info("Copied "+terminaltext.Clean(path)))
}
