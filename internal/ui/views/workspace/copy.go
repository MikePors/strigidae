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
	// Paths come from git; a path with control characters is never put on the
	// clipboard, where pasting it into a shell could run something.
	if termtext.Clean(path) != path {
		return ui.Error(fmt.Errorf("not copied: %s contains control characters", termtext.Clean(path)))
	}
	return tea.Batch(tea.SetClipboard(path), ui.Info("Copied "+termtext.Clean(path)))
}
