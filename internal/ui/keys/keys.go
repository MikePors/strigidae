// Package keys is the keymap registry: every key binding the app responds to
// is declared here (globals) or in the view that owns it, using bubbles/key,
// so the help overlay can be generated from one source of truth.
//
// Multi-key sequences are written as space-separated key names in a binding,
// for example key.WithKeys("g g") or key.WithKeys("] f"). The root model
// resolves key presses with a Matcher and delivers the result to the active
// view as a Msg; views test it with key.Matches(msg, binding) and never look
// at raw key strings.
package keys

import "charm.land/bubbles/v2/key"

// Msg is delivered to the active view when a key press, or a complete
// sequence, matches a binding that is active in that view. Seq is the key name
// ("j", "ctrl+d") or the space-joined sequence ("g g"). Msg implements
// fmt.Stringer so it works with key.Matches.
type Msg struct{ Seq string }

// String returns the matched key or sequence.
func (m Msg) String() string { return m.Seq }

// Global holds the bindings reserved across the whole app. Views use them
// through the Msg they receive; the root model itself handles Help and Command.
type Global struct {
	Down, Up             key.Binding
	Top, Bottom          key.Binding
	HalfDown, HalfUp     key.Binding
	PageDown, PageUp     key.Binding
	Select               key.Binding
	Open                 key.Binding
	Refresh              key.Binding
	Search               key.Binding
	NextMatch, PrevMatch key.Binding
	Cancel               key.Binding
	Help                 key.Binding
	// Command opens the command line (":q", ":qa"). The root handles it.
	Command key.Binding
}

// DefaultGlobal returns the default vim-style global bindings.
func DefaultGlobal() Global {
	b := func(keys, helpKey, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys), key.WithHelp(helpKey, desc))
	}
	return Global{
		Down:      b("j", "j", "down"),
		Up:        b("k", "k", "up"),
		Top:       b("g g", "gg", "top"),
		Bottom:    b("G", "G", "bottom"),
		HalfDown:  b("ctrl+d", "ctrl+d", "half page down"),
		HalfUp:    b("ctrl+u", "ctrl+u", "half page up"),
		PageDown:  b("ctrl+f", "ctrl+f", "page down"),
		PageUp:    b("ctrl+b", "ctrl+b", "page up"),
		Select:    b("enter", "enter", "open / select"),
		Open:      b("o", "o", "open in browser"),
		Refresh:   b("R", "R", "refresh"),
		Search:    b("/", "/", "filter / search"),
		NextMatch: b("n", "n", "next match"),
		PrevMatch: b("N", "N", "previous match"),
		Cancel:    b("esc", "esc", "cancel / clear"),
		Help:      b("?", "?", "toggle help"),
		Command:   b(":", ":", "command line (:q close, :qa quit)"),
	}
}

// Bindings returns every global binding as a flat list, for sequence matching.
func (g Global) Bindings() []key.Binding {
	var out []key.Binding
	for _, grp := range g.FullHelp() {
		out = append(out, grp...)
	}
	return out
}

// FullHelp groups the global bindings for the help overlay.
func (g Global) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{g.Down, g.Up, g.Top, g.Bottom, g.HalfDown, g.HalfUp, g.PageDown, g.PageUp},
		{g.Select, g.Open, g.Refresh, g.Search, g.NextMatch, g.PrevMatch},
		{g.Cancel, g.Help, g.Command},
	}
}
