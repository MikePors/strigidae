package workspace

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// copyFilename copies the selected file's path to the clipboard. The Files
// pane cursor and the diff pane's file are the same selection, so focus does
// not matter.
func (v *View) copyFilename() tea.Cmd {
	path := v.selectedPath()
	if path == "" {
		return ui.Error(errors.New("no file to copy"))
	}
	// Refuse control and bidi characters so the path cannot carry escapes or
	// line breaks (a pasted newline would submit a line). It is the raw path,
	// so it may still contain shell metacharacters.
	if termtext.Clean(path) != path {
		return ui.Error(fmt.Errorf("not copied: %s contains control characters", termtext.Clean(path)))
	}
	return tea.Batch(tea.SetClipboard(path), ui.Info("Copied "+termtext.Clean(path)))
}
