// Package keys is the keymap registry: every key binding the app responds to
// is declared here (globals) or in the view that owns it, using bubbles/key,
// so the help overlay can be generated from one source of truth.
//
// Multi-key sequences are written as space-separated key names in a binding,
// for example key.WithKeys("g g") or key.WithKeys("g f"). The root model
// resolves key presses with a Matcher and delivers the result to the active
// view as a Msg; views test it with key.Matches(msg, binding) and never look
// at raw key strings.
package keys

import "charm.land/bubbles/v2/key"

// Message is delivered to the active view when a key press, or a complete
// sequence, matches a binding that is active in that view. Seq is the key name
// ("j", "ctrl+d") or the space-joined sequence ("g g"). Msg implements
// fmt.Stringer so it works with key.Matches.
//
// Message is not a tea.KeyPressMsg: embedded bubbles components (viewport, ...)
// ignore it. Views drive them through their methods (ScrollDown, GotoTop, ...)
// in response to a Msg and never forward it to the component's Update.
type Message struct{ Sequence string }

// String returns the matched key or sequence.
func (message Message) String() string { return message.Sequence }

// Global holds the bindings reserved across the whole app. Views use them
// through the Msg they receive; the root model itself handles Help and Command.
type Global struct {
	Down, Up                 key.Binding
	Top, Bottom              key.Binding
	HalfDown, HalfUp         key.Binding
	PageDown, PageUp         key.Binding
	Select                   key.Binding
	Open                     key.Binding
	Refresh                  key.Binding
	Search                   key.Binding
	NextMatch, PreviousMatch key.Binding
	Cancel                   key.Binding
	Help                     key.Binding
	// Command opens the command line (":q", ":qa"). The root handles it.
	Command key.Binding
}

// DefaultGlobal returns the default vim-style global bindings.
func DefaultGlobal() Global {
	bind := func(keys, helpKey, description string) key.Binding {
		return key.NewBinding(key.WithKeys(keys), key.WithHelp(helpKey, description))
	}
	return Global{
		Down:          bind("j", "j", "down"),
		Up:            bind("k", "k", "up"),
		Top:           bind("g g", "gg", "top"),
		Bottom:        bind("G", "G", "bottom"),
		HalfDown:      bind("ctrl+d", "ctrl+d", "half page down"),
		HalfUp:        bind("ctrl+u", "ctrl+u", "half page up"),
		PageDown:      bind("ctrl+f", "ctrl+f", "page down"),
		PageUp:        bind("ctrl+b", "ctrl+b", "page up"),
		Select:        bind("enter", "enter", "open / select"),
		Open:          bind("o", "o", "open in browser"),
		Refresh:       bind("R", "R", "refresh"),
		Search:        bind("/", "/", "filter / search"),
		NextMatch:     bind("n", "n", "next match"),
		PreviousMatch: bind("N", "N", "previous match"),
		Cancel:        bind("esc", "esc", "cancel / clear"),
		Help:          bind("?", "?", "toggle help"),
		Command:       bind(":", ":", "command line (:q close, :qa quit)"),
	}
}

// Bindings returns every global binding as a flat list, for sequence matching.
func (global Global) Bindings() []key.Binding {
	var out []key.Binding
	for _, bindings := range global.FullHelp() {
		out = append(out, bindings...)
	}
	return out
}

// FullHelp groups the global bindings for the help overlay.
func (global Global) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{global.Down, global.Up, global.Top, global.Bottom, global.HalfDown, global.HalfUp, global.PageDown, global.PageUp},
		{global.Select, global.Open, global.Refresh, global.Search, global.NextMatch, global.PreviousMatch},
		{global.Cancel, global.Help, global.Command},
	}
}
