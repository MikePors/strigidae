package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui/components/picker"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

func (v *View) openPicker() {
	files := v.files()
	if len(files) == 0 {
		return
	}
	items := make([]string, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		items[i] = termtext.Clean(f.Path)
		paths[i] = f.Path
	}
	v.pickerItems, v.pickerPaths = items, paths
	v.picker = picker.New(v.th, "Go to file", items)
}

// pickerKey feeds a raw key to the open picker.
func (v *View) pickerKey(msg tea.KeyPressMsg) {
	res := v.picker.Update(msg)
	switch res.Outcome {
	case picker.Cancelled:
		v.picker = nil
	case picker.Chosen:
		v.picker = nil
		path := v.pickerPaths[res.Index]
		for i, f := range v.files() {
			if f.Path == path {
				v.selectEntry(i + 1)
			}
		}
	case picker.Open:
	}
}
