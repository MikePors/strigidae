package keys

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// namedKeys are the keys that have a name rather than a character.
var namedKeys = []string{
	"esc", "enter", "tab", "backspace", "space", "delete", "insert",
	"up", "down", "left", "right", "home", "end", "pgup", "pgdown",
	"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
}

// CheckKeys reports why keys cannot be the keys of one action, or nil. Each
// entry is a key or a space-separated sequence of keys, in the notation that
// internal/userinterface/CLAUDE.md documents.
func CheckKeys(sequences []string) error {
	if len(sequences) == 0 {
		return fmt.Errorf("an action needs at least one key")
	}
	for _, sequence := range sequences {
		if err := checkSequence(sequence); err != nil {
			return err
		}
	}
	return nil
}

func checkSequence(sequence string) error {
	if sequence == "" || strings.TrimSpace(sequence) != sequence || strings.Contains(sequence, "  ") {
		return fmt.Errorf("key %q must be keys separated by single spaces", sequence)
	}
	for position, pressed := range strings.Split(sequence, " ") {
		if err := checkKey(pressed); err != nil {
			return fmt.Errorf("key %q: %w", sequence, err)
		}
		// 1 to 9 at the start of a binding begin a count; 0 only continues one.
		if position == 0 && len(pressed) == 1 && pressed[0] >= '1' && pressed[0] <= '9' {
			return fmt.Errorf("key %q: the digits 1 to 9 at the start of a binding begin a count", sequence)
		}
	}
	return nil
}

// sameByte maps the control keys that terminals send as the same byte as a
// named key; the named key is what strig would see, so the control spelling
// would never match.
var sameByte = map[string]string{"ctrl+i": "tab", "ctrl+m": "enter", "ctrl+[": "esc"}

// checkKey accepts one key press: a character, a named key, or either with
// ctrl+ and alt+ in front, in that order. shift+ is written only before a named
// key (shift+tab); a shifted character is the character itself (G, not shift+g).
func checkKey(pressed string) error {
	if named, found := sameByte[pressed]; found {
		return fmt.Errorf("%q is sent by terminals as %q; write %q", pressed, named, named)
	}
	base := pressed
	modified, shifted := false, false
	for _, modifier := range []string{"ctrl+", "alt+"} {
		if rest, found := strings.CutPrefix(base, modifier); found && rest != "" {
			base, modified = rest, true
		}
	}
	if rest, found := strings.CutPrefix(base, "shift+"); found && rest != "" {
		base, shifted = rest, true
	}
	switch {
	case slices.Contains(namedKeys, base):
		return nil
	case shifted:
		return fmt.Errorf("write a shifted character as the character itself, not %q (shift+ goes only before a named key such as tab)", pressed)
	case utf8.RuneCountInString(base) != 1:
		return fmt.Errorf("%q is not a key (a character, or one of: %s, optionally after ctrl+ or alt+)", pressed, strings.Join(namedKeys, ", "))
	}
	character, _ := utf8.DecodeRuneInString(base)
	switch {
	case !unicode.IsPrint(character) || unicode.IsSpace(character):
		return fmt.Errorf("%q is not a key (write a space as space)", pressed)
	case modified && unicode.IsUpper(character):
		return fmt.Errorf("%q is not a key (after ctrl+ or alt+ write the lower-case letter)", pressed)
	}
	return nil
}

// singleKeyActions are the actions the shell matches on one key press at a
// time (the help overlay and the command line are open while they act), so
// they cannot be a sequence yet.
var singleKeyActions = []string{"help", "cancel", "command-line"}

// Override replaces the keys of the global action called name with sequences,
// and shows them in the help text. Actions that are not named keep their keys.
func (global *Global) Override(name string, sequences []string) error {
	actions := global.Actions()
	for _, action := range actions {
		if action.Name != name {
			continue
		}
		if err := CheckKeys(sequences); err != nil {
			return fmt.Errorf("action %q: %w", name, err)
		}
		if slices.Contains(singleKeyActions, name) {
			for _, sequence := range sequences {
				if strings.Contains(sequence, " ") {
					return fmt.Errorf("action %q: key %q must be a single key, not a sequence", name, sequence)
				}
			}
		}
		action.Binding.SetKeys(sequences...)
		action.Binding.SetHelp(helpKeys(sequences), action.Binding.Help().Desc)
		return nil
	}
	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = action.Name
	}
	return fmt.Errorf("unknown action %q (valid actions: %s)", name, strings.Join(names, ", "))
}

// helpKeys writes sequences the way the help overlay shows them: a sequence
// without its spaces ("gg"), alternatives joined by a slash ("j/down").
func helpKeys(sequences []string) string {
	shown := make([]string, len(sequences))
	for i, sequence := range sequences {
		shown[i] = strings.ReplaceAll(sequence, " ", "")
	}
	return strings.Join(shown, "/")
}
