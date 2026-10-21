package workspace

import (
	"github.com/MikePors/strigidae/internal/github"
)

// files returns the changed files, or nil before the pull request loads.
func (v *View) files() []github.ChangedFile {
	if v.detail == nil {
		return nil
	}
	return v.detail.Files
}

// entries is the number of rows in the Files pane: Overview plus the files.
func (v *View) entries() int { return 1 + len(v.files()) }

// sel is the selected Files row: 0 is Overview, i is Files[i-1].
func (v *View) sel() int { return v.fw.Cursor() }

// setSel moves the selection to row i, scrolling the Files list to show it.
func (v *View) setSel(i int) { v.fw.Restore(i, v.fw.Top(), 0) }

func (v *View) selectedPath() string {
	if s := v.sel(); s > 0 && s <= len(v.files()) {
		return v.files()[s-1].Path
	}
	return ""
}

func (v *View) selectionChanged() {
	if s := v.sel(); s > 0 && s <= len(v.files()) {
		f := v.files()[s-1]
		if f != v.paneFile {
			v.paneFile = f
			v.filePane.SetFile(f)
		}
	}
}

func (v *View) selectEntry(i int) {
	prev := v.sel()
	v.setSel(i)
	if v.sel() != prev {
		v.selectionChanged()
	}
	v.layout()
}
