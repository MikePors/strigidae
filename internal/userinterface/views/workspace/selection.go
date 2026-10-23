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

// entries is the number of rows in the Files pane: Overview plus the files.
func (view *View) entries() int { return 1 + len(view.files()) }

// selection is the selected Files row: 0 is Overview, i is Files[i-1].
func (view *View) selection() int { return view.window.Cursor() }

// setSelection moves the selection to row i, scrolling the Files list to show it.
func (view *View) setSelection(i int) { view.window.Restore(i, view.window.Top(), 0) }

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

func (view *View) selectEntry(i int) {
	previous := view.selection()
	view.setSelection(i)
	if view.selection() != previous {
		view.selectionChanged()
	}
	view.layout()
}
