package workspace

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// verdictWords name the verdicts of `:review`, which has no command of its own
// per verdict.
var verdictWords = map[string]github.ReviewEvent{
	"approve":         github.ReviewEventApprove,
	"request-changes": github.ReviewEventRequestChanges,
	"comment":         github.ReviewEventComment,
}

// verdictWordList is the words as an error message lists them.
const verdictWordList = "approve, request-changes or comment"

// reviewArguments is what the text after a review command asks for.
type reviewArguments struct {
	// event is the verdict `:review` was given; empty when it was not, or for
	// the commands that have a verdict of their own.
	event github.ReviewEvent
	// message is the summary given with -m; hasMessage tells an empty message
	// (-m "") from none, which opens the editor.
	message    string
	hasMessage bool
}

// parseReviewArguments reads `[-m <message>]`, preceded by a verdict word when
// verdictFirst is set (`:review`). The message is the rest of the line, with one
// pair of surrounding quotes removed.
func parseReviewArguments(arguments string, verdictFirst bool) (reviewArguments, error) {
	var parsed reviewArguments
	rest := strings.TrimSpace(arguments)
	if verdictFirst && rest != "" && !strings.HasPrefix(rest, "-") {
		word, remainder, _ := strings.Cut(rest, " ")
		event, ok := verdictWords[word]
		if !ok {
			return parsed, fmt.Errorf("unknown verdict %q (use %s)", terminaltext.Clean(word), verdictWordList)
		}
		parsed.event = event
		rest = strings.TrimSpace(remainder)
	}
	if rest == "" {
		return parsed, nil
	}
	flag, message, _ := strings.Cut(rest, " ")
	if flag != "-m" {
		return parsed, fmt.Errorf("unknown argument %q (use -m <message>)", terminaltext.Clean(flag))
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return parsed, errors.New(`-m needs a message (-m "" sends none)`)
	}
	if verdictFirst && parsed.event == "" {
		return parsed, fmt.Errorf("-m needs a verdict first (%s)", verdictWordList)
	}
	parsed.message, parsed.hasMessage = unquote(message), true
	return parsed, nil
}

// unquote removes one pair of matching quotes around text.
func unquote(text string) string {
	if len(text) >= 2 && (text[0] == '"' || text[0] == '\'') && text[len(text)-1] == text[0] {
		return text[1 : len(text)-1]
	}
	return text
}

// checkVerdictArguments refuses bad arguments of a command that has its own
// verdict (`:approve`, `:request-changes`).
func checkVerdictArguments(arguments string) tea.Cmd {
	if _, err := parseReviewArguments(arguments, false); err != nil {
		return userinterface.Error(err)
	}
	return nil
}

// checkReviewArguments refuses bad arguments of `:review`.
func checkReviewArguments(arguments string) tea.Cmd {
	if _, err := parseReviewArguments(arguments, true); err != nil {
		return userinterface.Error(err)
	}
	return nil
}

// command runs `:approve` and `:request-changes`: with -m the editor is
// skipped.
func (currentFlow *reviewFlow) command(environment reviewEnvironment, event github.ReviewEvent, arguments string) tea.Cmd {
	parsed, err := parseReviewArguments(arguments, false)
	if err != nil {
		return userinterface.Error(err)
	}
	if parsed.hasMessage {
		return currentFlow.startWithMessage(environment, event, parsed.message)
	}
	return currentFlow.start(environment, event)
}

// reviewCommand runs `:review`: without arguments the chooser opens, with a
// verdict that verdict's review starts.
func (currentFlow *reviewFlow) reviewCommand(environment reviewEnvironment, arguments string) tea.Cmd {
	parsed, err := parseReviewArguments(arguments, true)
	if err != nil {
		return userinterface.Error(err)
	}
	switch {
	case parsed.event == "":
		return currentFlow.openChooser(environment)
	case parsed.hasMessage:
		return currentFlow.startWithMessage(environment, parsed.event, parsed.message)
	default:
		return currentFlow.start(environment, parsed.event)
	}
}
