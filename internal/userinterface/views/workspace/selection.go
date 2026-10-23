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

// sel is the selected Files row: 0 is Overview, i is Files[i-1].
func (view *View) sel() int { return view.fw.Cursor() }

// setSel moves the selection to row i, scrolling the Files list to show it.
func (view *View) setSel(i int) { view.fw.Restore(i, view.fw.Top(), 0) }

func (view *View) selectedPath() string {
	if size := view.sel(); size > 0 && size <= len(view.files()) {
		return view.files()[size-1].Path
	}
	return ""
}

func (view *View) selectionChanged() {
	size := view.sel()
	if size == 0 || size > len(view.files()) {
		// The one place a line selection ends when the file leaves the screen,
		// including when a reload drops the file. (Another file or a reloaded
		// diff ends it in the diff component.)
		view.filePane.VisualEnd()
		return
	}
	file := view.files()[size-1]
	if file != view.paneFile {
		view.paneFile = file
		view.filePane.SetFile(file)
	}
}

func (view *View) selectEntry(i int) {
	previous := view.sel()
	view.setSel(i)
	if view.sel() != previous {
		view.selectionChanged()
	}
	view.layout()
}
