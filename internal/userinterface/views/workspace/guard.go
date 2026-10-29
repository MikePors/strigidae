package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
)

// writeKind is the kind of write an action makes to GitHub. Every action that
// writes declares its kind and asks the guard before it starts, and again at
// each point where it is re-admitted (after a yes, or after a re-read).
// Actions that write nothing to GitHub (`m`, folds, navigation,
// `:copy-filename`, `:checks`) have no kind and no guard.
type writeKind int

const (
	// writeComment: `c` (new comment, top-level or inline), `V` (starts an
	// inline comment), `r`, `e` / `:edit-comment`, `D` / `:delete-comment`.
	writeComment writeKind = iota
	// writeThread: `x`, `:resolve`, `:unresolve`.
	writeThread
	// writeReaction: `:react`.
	writeReaction
	// writeReview: `:approve`, `:request-changes`, `:review`.
	writeReview
	// writeMerge: `:merge`.
	writeMerge
	// writeDiscard: `:discard-review`.
	writeDiscard
)

// busyStates says which busy states block a kind of write.
type busyStates struct {
	submittingReview bool // a review is being submitted
	mergeInProgress  bool // a merge is in progress
	commentSaving    bool // a comment write is in flight
}

// allBusy blocks a write in every busy state, the first version of the table.
var allBusy = busyStates{submittingReview: true, mergeInProgress: true, commentSaving: true}

// guardTable is the one place that says what blocks each kind of write. Keying
// it by target (reacting to a published comment during a submit, say) is
// future work.
var guardTable = map[writeKind]busyStates{
	writeComment:  allBusy,
	writeThread:   allBusy,
	writeReaction: allBusy,
	writeReview:   allBusy,
	writeMerge:    allBusy,
	writeDiscard:  allBusy,
}

// guardState is what the guard looks at, at one moment.
type guardState struct {
	loaded           bool // the pull request detail has loaded
	diffReady        bool // a diff is loaded and shown
	submittingReview bool // a review submission is in flight
	mergeInProgress  bool // a merge is in flight
	commentSaving    bool // a comment write is in flight
}

// ready returns the notice to show when the pull request or its diff is not
// loaded, or nil. It comes first, so a notice about what is missing at the
// cursor never stands in for "not loaded".
func (state guardState) ready() tea.Cmd {
	switch {
	case !state.loaded:
		return userinterface.Info("pull request not loaded yet")
	case !state.diffReady:
		return userinterface.Info("diff not loaded")
	}
	return nil
}

// busy returns the notice to show when a busy state blocks the kind of write,
// or nil.
func (state guardState) busy(kind writeKind) tea.Cmd {
	blocks := guardTable[kind]
	switch {
	case blocks.submittingReview && state.submittingReview:
		return userinterface.Info("a review is being submitted")
	case blocks.mergeInProgress && state.mergeInProgress:
		return userinterface.Info("a merge is in progress")
	case blocks.commentSaving && state.commentSaving:
		return userinterface.Info(commentSavingText)
	}
	return nil
}

// admit is ready followed by busy, for actions with no target to resolve in
// between.
func (state guardState) admit(kind writeKind) tea.Cmd {
	if notice := state.ready(); notice != nil {
		return notice
	}
	return state.busy(kind)
}

// guardState reads the guard's inputs from the view.
func (view *View) guardState() guardState {
	return guardState{
		loaded:           view.detail != nil,
		diffReady:        !view.diffPending && view.diff.Parsed != nil && view.diff.Err == nil,
		submittingReview: view.review.submitting,
		mergeInProgress:  view.merge.merging,
		commentSaving:    view.writes > 0,
	}
}
