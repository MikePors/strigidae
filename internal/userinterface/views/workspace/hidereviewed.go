package workspace

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
)

// With the hidereviewed option, hunks marked reviewed leave the diff pane:
// each is drawn as one placeholder row. A hunk marked while the option is on
// first fades for a grace period, like a viewed file under hideviewed.

const hunkFadeKeyPrefix = "reviewed:"

// hunkFadeKey names a hunk's fade by its path and fingerprint, so it follows
// the hunk through a refresh.
func (view *View) hunkFadeKey(file *diff.File, hunk int) string {
	return hunkFadeKeyPrefix + file.Path() + "\x00" + view.hunkKey(file, hunk).fingerprint
}

// hunkHidden reports whether the diff pane leaves a hunk out: reviewed, the
// option on, and not in its grace period.
func (view *View) hunkHidden(file *diff.File, hunk int) bool {
	return view.options.HideReviewed() && view.hunkReviewed(file, hunk) && !view.fades.active(view.hunkFadeKey(file, hunk))
}

// hunkFade returns how far a hunk has faded.
func (view *View) hunkFade(file *diff.File, hunk int) int {
	return view.fades.step(view.hunkFadeKey(file, hunk))
}

// hunkMarkChanged starts the fade of a hunk just marked reviewed, or ends it
// for one unmarked, which then stays (and comes back if it was hidden).
func (view *View) hunkMarkChanged(file *diff.File, hunk int, reviewed bool) tea.Cmd {
	key := view.hunkFadeKey(file, hunk)
	if !reviewed {
		view.fades.cancel(key)
		view.filePane.HiddenHunksChanged()
		return nil
	}
	if !view.options.HideReviewed() {
		return nil
	}
	return view.fadeCommand(key, view.fades.begin(key, view.now()))
}

// hunkFadeTicked handles a timer of a hunk's fade: the hunk is hidden once
// it is over.
func (view *View) hunkFadeTicked(message fadeMessage) tea.Cmd {
	current, over := view.fades.advance(message.key, message.generation, view.now())
	switch {
	case !current:
		return nil
	case over:
		view.filePane.HiddenHunksChanged()
		return nil
	}
	return view.fadeCommand(message.key, message.generation)
}
