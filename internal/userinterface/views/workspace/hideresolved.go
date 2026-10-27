package workspace

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
)

// With the hideresolved option, resolved threads leave the diff pane. One
// resolved from strig while the option is on first fades for a grace period,
// like a hunk marked reviewed under hidereviewed; one that arrives resolved
// hides at once.

const threadFadeKeyPrefix = "resolved:"

func threadFadeKey(identifier string) string { return threadFadeKeyPrefix + identifier }

// threadFade reports whether a thread is fading and how far.
func (view *View) threadFade(identifier string) (step int, fading bool) {
	key := threadFadeKey(identifier)
	return view.fades.step(key), view.fades.active(key)
}

// threadResolutionChanged starts the fade of a thread just resolved, or ends
// it for one unresolved, which then stays.
func (view *View) threadResolutionChanged(identifier string, resolved bool) tea.Cmd {
	key := threadFadeKey(identifier)
	if !resolved {
		view.fades.cancel(key)
		return nil
	}
	if !view.options.HideResolved() {
		return nil
	}
	return view.fadeCommand(key, view.fades.begin(key, view.now()))
}

// threadFadeTicked handles a timer of a thread's fade: the thread leaves once
// it is over. A thread unresolved meanwhile (by a refresh) stops fading.
func (view *View) threadFadeTicked(message fadeMessage, identifier string) tea.Cmd {
	current, over := view.fades.advance(message.key, message.generation, view.now())
	switch {
	case !current:
		return nil
	case over:
		view.filePane.ThreadFadesChanged()
		return nil
	case !view.stillResolved(identifier):
		view.fades.cancel(message.key)
		return nil
	}
	return view.fadeCommand(message.key, message.generation)
}

func (view *View) stillResolved(identifier string) bool {
	index := slices.IndexFunc(view.threads, func(thread github.ReviewThread) bool { return thread.Identifier == identifier })
	return index >= 0 && view.threads[index].IsResolved
}
