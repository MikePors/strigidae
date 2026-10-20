// Package ui is the app shell: the root Bubble Tea model, the view stack,
// navigation and status messages, the status bar, and the help overlay.
//
// Screens implement View and are composed by the root Model. Views never
// touch the stack; they return commands built by Push, Pop, Quit, Info, Error
// and Loading, and the root acts on the resulting messages.
package ui

import (
	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
)

// View is a screen held by the root model's view stack.
//
// The root forwards to the active (top) view:
//   - keys.Msg for key presses and sequences that match a binding active in
//     the view (the global bindings plus anything in FullHelp). Test with
//     key.Matches(msg, binding).
//   - raw tea.KeyPressMsg for keys that match nothing, and for every key
//     while the view implements KeyCapturer and is capturing.
//   - tea.WindowSizeMsg with the area the view may draw in (the terminal
//     minus the status bar), on push and on every resize. Views below the top
//     of the stack get it too.
//   - ThemeMsg when the theme changes.
//   - every other message (command results, ticks); only the top view sees them.
//
// The root handles Back (q), Help (?) and Quit (ctrl+c) itself and never
// forwards them; Cancel (esc) is always forwarded.
//
// ShortHelp and FullHelp come from help.KeyMap. List only bindings specific
// to the view; the global ones are added by the root. FullHelp doubles as the
// registry of the view's multi-key sequences (bindings with keys like "] f"),
// so every binding the view reacts to must appear there, even if ShortHelp
// leaves it out.
type View interface {
	// Init returns the command to run when the view is pushed, or nil.
	Init() tea.Cmd
	// Update handles a message and returns the updated view. It must not do I/O.
	Update(msg tea.Msg) (View, tea.Cmd)
	// Render draws the view in at most width columns and height rows. The
	// root clips anything beyond that.
	Render(width, height int) string
	// Title is the short name shown in the status bar breadcrumbs.
	Title() string

	help.KeyMap
}

// KeyCapturer is optionally implemented by views with a focused text input.
// While CapturesKeys returns true the root forwards every key, other than
// ctrl+c, as a raw tea.KeyPressMsg without interpreting it.
type KeyCapturer interface {
	CapturesKeys() bool
}
