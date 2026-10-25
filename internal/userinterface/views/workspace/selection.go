package workspace

import (
	"github.com/MikePors/strigidae/internal/github"
)

// files returns the changed files, or nil before the pull request loads.
func (view *View) files() []github.ChangedFile {
	if view.detail == nil {
		return nil
	}
	return view.detail.Files
}

// selection is the entry the right pane shows: 0 is Overview, i is Files[i-1].
// It is the cursor's entry unless the file picker opened a hidden file.
func (view *View) selection() int { return view.current }

func (view *View) selectedPath() string {
	if selected := view.selection(); selected > 0 && selected <= len(view.files()) {
		return view.files()[selected-1].Path
	}
	return ""
}

func (view *View) selectionChanged() {
	selected := view.selection()
	if selected == 0 || selected > len(view.files()) {
		// The one place a line selection ends when the file leaves the screen,
		// including when a reload drops the file. (Another file or a reloaded
		// diff ends it in the diff component.)
		view.filePane.VisualEnd()
		return
	}
	file := view.files()[selected-1]
	if file != view.paneFile {
		view.paneFile = file
		view.filePane.SetFile(file)
	}
}

// selectEntry shows entry i and puts the cursor on its row, or the nearest
// one when the file is hidden.
func (view *View) selectEntry(i int) {
	previous := view.current
	view.current = i
	view.window.Restore(nearestRow(view.rows, i), view.window.Top(), 0)
	if view.current != previous {
		view.selectionChanged()
	}
	view.layout()
}
