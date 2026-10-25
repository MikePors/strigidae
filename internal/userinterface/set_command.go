package userinterface

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// setRequestedMessage asks the root to apply a `:set` line.
type setRequestedMessage struct{ arguments string }

// setCommand is `:set`: it reaches the root as a message because the root
// owns the option values.
func setCommand(arguments string) tea.Cmd {
	return messageCommand(setRequestedMessage{arguments: arguments})
}

// applySet applies the words of a `:set` line to current. `name` turns an
// option on, `noname` turns it off, `name?` shows it, and no words show all.
// A line either changes options or asks about them, not both. It returns the
// new values, the text to show, and whether anything was set. On any error
// nothing changes: the returned values are current.
func applySet(arguments string, current options.Values) (updated options.Values, text string, changed bool, err error) {
	words := strings.Fields(arguments)
	if len(words) == 0 {
		return current, current.String(), false, nil
	}
	updated = current
	var shown []string
	queries := 0
	for _, word := range words {
		name, query, value, wordErr := parseSetWord(word)
		if wordErr != nil {
			return current, "", false, wordErr
		}
		if query {
			queries++
		} else {
			updated = updated.With(name, value)
		}
		shown = append(shown, name)
	}
	if queries > 0 && queries < len(words) {
		return current, "", false, errors.New("can't mix name? with changes in one :set line")
	}
	descriptions := make([]string, len(shown))
	for i, name := range shown {
		descriptions[i] = updated.Describe(name)
	}
	return updated, strings.Join(descriptions, " "), queries == 0, nil
}

// parseSetWord splits one word into an option name and what to do with it.
func parseSetWord(word string) (name string, query, value bool, err error) {
	if name, found := strings.CutSuffix(word, "?"); found {
		if name == "" {
			return "", false, false, errors.New("missing option name")
		}
		if _, ok := options.Lookup(name); !ok {
			return "", false, false, unknownOption(name)
		}
		return name, true, false, nil
	}
	if name, _, hasValue := strings.Cut(word, "="); hasValue {
		bare := strings.TrimPrefix(name, "no")
		if _, ok := options.Lookup(bare); ok {
			return "", false, false, fmt.Errorf("%s is on or off: use :set %s or :set no%s", bare, bare, bare)
		}
		return "", false, false, unknownOption(name)
	}
	if _, ok := options.Lookup(word); ok {
		return word, false, true, nil
	}
	if name, found := strings.CutPrefix(word, "no"); found {
		if _, ok := options.Lookup(name); ok {
			return name, false, false, nil
		}
	}
	return "", false, false, unknownOption(word)
}

func unknownOption(name string) error {
	return fmt.Errorf("unknown option: %s", terminaltext.Clean(name))
}

// set handles a `:set` line: it changes the options for this session (the
// config file is never written), tells every view, and shows the result.
func (model *Model) set(arguments string) tea.Cmd {
	updated, text, changed, err := applySet(arguments, model.options)
	if err != nil {
		return Error(err)
	}
	if !changed {
		return Info(text)
	}
	model.options = updated
	return tea.Batch(model.broadcast(OptionsMessage{Values: updated}), Info(text))
}
