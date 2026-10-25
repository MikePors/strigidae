package userinterface

import (
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
// It returns the new values and the text to show. On any error nothing
// changes: the returned values are current.
func applySet(arguments string, current options.Values) (updated options.Values, text string, err error) {
	words := strings.Fields(arguments)
	if len(words) == 0 {
		return current, current.String(), nil
	}
	updated = current
	var shown []string
	for _, word := range words {
		name, query, value, wordErr := parseSetWord(word)
		if wordErr != nil {
			return current, "", wordErr
		}
		if !query {
			updated = updated.With(name, value)
		}
		shown = append(shown, name)
	}
	descriptions := make([]string, len(shown))
	for i, name := range shown {
		descriptions[i] = updated.Describe(name)
	}
	return updated, strings.Join(descriptions, " "), nil
}

// parseSetWord splits one word into an option name and what to do with it.
func parseSetWord(word string) (name string, query, value bool, err error) {
	if name, found := strings.CutSuffix(word, "?"); found {
		if _, ok := options.Lookup(name); !ok {
			return "", false, false, unknownOption(name)
		}
		return name, true, false, nil
	}
	if name, _, hasValue := strings.Cut(word, "="); hasValue {
		if _, ok := options.Lookup(strings.TrimPrefix(name, "no")); ok {
			return "", false, false, fmt.Errorf("%s is on or off: use :set %s or :set no%s", terminaltext.Clean(name), name, name)
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
	updated, text, err := applySet(arguments, model.options)
	if err != nil {
		return Error(err)
	}
	model.options = updated
	return tea.Batch(model.broadcast(OptionsMessage{Values: updated}), Info(text))
}
