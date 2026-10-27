package workspace

import tea "charm.land/bubbletea/v2"

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

// beginThreadFade starts the fade of a thread just resolved.
func (view *View) beginThreadFade(identifier string) tea.Cmd {
	if !view.options.HideResolved() {
		return nil
	}
	key := threadFadeKey(identifier)
	return view.fadeCommand(key, view.fades.begin(key, view.now()))
}

// holdThreadFade keeps a thread just unresolved shown at its normal look
// while the screen still holds it resolved, because a reload is in flight;
// the reload's arrival ends the hold.
func (view *View) holdThreadFade(identifier string) {
	if view.options.HideResolved() {
		view.fades.hold(threadFadeKey(identifier))
	}
}

// endUnresolvedFades stops the fades of the threads now shown unresolved,
// which then stay. It reads view.threads, so callers run it only once that
// holds the latest data: while a reload is in flight the list may be older
// than a write, and the reload's arrival calls it again.
func (view *View) endUnresolvedFades() {
	for _, thread := range view.threads {
		if !thread.IsResolved {
			view.fades.cancel(threadFadeKey(thread.Identifier))
		}
	}
}

// threadFadeTicked handles a timer of a thread's fade: the thread leaves once
// it is over.
func (view *View) threadFadeTicked(message fadeMessage) tea.Cmd {
	current, over := view.fades.advance(message.key, message.generation, view.now())
	switch {
	case !current:
		return nil
	case over:
		view.filePane.ThreadFadesChanged()
		return nil
	}
	return view.fadeCommand(message.key, message.generation)
}
