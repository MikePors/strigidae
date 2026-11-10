package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
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
	view.pickerItems, view.pickerPaths, view.pickerChecks, view.pickerReaction = items, paths, nil, nil
	view.picker = picker.New(view.theme, "Go to file", items)
}

// pickerKey feeds a raw key to the open picker. Choosing a check opens its
// page in the browser; choosing a file selects it.
func (view *View) pickerKey(message tea.KeyPressMsg) tea.Cmd {
	result := view.picker.Update(message)
	switch result.Outcome {
	case picker.Cancelled:
		view.picker, view.pickerChecks, view.pickerReaction, view.pickerComment = nil, nil, nil, nil
	case picker.Chosen:
		checks, target, selection := view.pickerChecks, view.pickerReaction, view.pickerComment
		view.picker, view.pickerChecks, view.pickerReaction, view.pickerComment = nil, nil, nil, nil
		if selection != nil {
			selection.call.target.comment = selection.targets[result.Index]
			return view.invoke(selection.action, selection.call)
		}
		if target != nil {
			return view.invoke(view.entryNamed("react"), invocation{arguments: github.ReactionContents[result.Index].Shortcode(), target: resolvedTarget{comment: *target}})
		}
		if checks != nil {
			return view.openCheck(checks[result.Index])
		}
		path := view.pickerPaths[result.Index]
		for i, file := range view.files() {
			if file.Path == path {
				view.selectEntry(i + 1)
			}
		}
	case picker.Open:
	}
	return nil
}
