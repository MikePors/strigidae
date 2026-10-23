package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

func (view *View) openPicker() {
	files := view.files()
	if len(files) == 0 {
		return
	}
	items := make([]string, len(files))
	paths := make([]string, len(files))
	for i, file := range files {
		items[i] = terminaltext.Clean(file.Path)
		paths[i] = file.Path
	}
	view.pickerItems, view.pickerPaths = items, paths
	view.picker = picker.New(view.theme, "Go to file", items)
}

// pickerKey feeds a raw key to the open picker.
func (view *View) pickerKey(message tea.KeyPressMsg) {
	result := view.picker.Update(message)
	switch result.Outcome {
	case picker.Cancelled:
		view.picker = nil
	case picker.Chosen:
		view.picker = nil
		path := view.pickerPaths[result.Index]
		for i, file := range view.files() {
			if file.Path == path {
				view.selectEntry(i + 1)
			}
		}
	case picker.Open:
	}
}
