package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
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
		view.picker, view.pickerChecks, view.pickerReaction = nil, nil, nil
		view.pickerReactionTargets, view.pickerReactionName = nil, ""
	case picker.Chosen:
		checks, target := view.pickerChecks, view.pickerReaction
		reactionTargets, reactionName := view.pickerReactionTargets, view.pickerReactionName
		view.picker, view.pickerChecks, view.pickerReaction = nil, nil, nil
		view.pickerReactionTargets, view.pickerReactionName = nil, ""
		if len(reactionTargets) > 0 {
			if notice := view.guardState().admit(writeOverviewReaction); notice != nil {
				return notice
			}
			selected := reactionTargets[result.Index]
			fresh, found := view.resolveReactionTarget(selected.identifier)
			if !found {
				return userinterface.Info("comment no longer exists; refresh and try again")
			}
			freshTarget := reactionTarget{commentIdentifier: fresh.identifier, reactions: fresh.reactions, kind: fresh.kind}
			if reactionName != "" {
				return view.toggleReaction(freshTarget, reactionName)
			}
			view.openReactionPicker(freshTarget, fresh.author)
			return nil
		}
		if target != nil {
			// Choosing is the re-admission point: a review or merge may
			// have started while the picker was open.
			kind := writeReaction
			if target.kind != commentReview {
				kind = writeOverviewReaction
			}
			if notice := view.guardState().admit(kind); notice != nil {
				return notice
			}
			fresh, found := view.resolveReactionTarget(target.commentIdentifier)
			if !found {
				return userinterface.Info("comment no longer exists; refresh and try again")
			}
			return view.toggleReaction(reactionTarget{commentIdentifier: fresh.identifier, reactions: fresh.reactions, kind: fresh.kind}, github.ReactionContents[result.Index])
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
