package userinterface

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/pullrequestmarks"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// MarksMessage gives views every mark, after any of them changed and to each
// view as it is pushed. A view keeps the last one it was sent and derives what
// it draws from it. Marks are a value, so nothing is shared with the root.
type MarksMessage struct{ Marks pullrequestmarks.Marks }

// SetMarkMessage asks the root to give a pull request a mark, replacing any
// it had. A list sends it for `:mark`.
type SetMarkMessage struct {
	Key  pullrequestmarks.Key
	Text string
}

// ClearMarkMessage asks the root to remove a pull request's mark. A list sends
// it for `:unmark`, after checking that the pull request has one.
type ClearMarkMessage struct{ Key pullrequestmarks.Key }

// marksSavedMessage is the result of the save in flight.
type marksSavedMessage struct{ err error }

// marksPart is the root's slice of state for pull request marks: the marks, the
// file they are saved in, and the bookkeeping of saves. The root hands it its
// messages in one line and keeps nothing else about marks.
type marksPart struct {
	marks pullrequestmarks.Marks
	// path is the file the marks are saved in; empty keeps them for the
	// session only.
	path string
	// unreadable is why the file could not be read at startup. The file is
	// then never written, so it is not overwritten before the user has looked.
	unreadable error
	// saving is true while a save is in flight. There is at most one, so an
	// older copy can never land on disk after a newer one.
	saving bool
	// pending is true when marks changed during a save: when it ends, one save
	// of the then-latest marks follows.
	pending bool
}

func newMarksPart(startup Options) marksPart {
	part := marksPart{marks: startup.Marks, unreadable: startup.MarksError}
	if startup.StateDirectory != "" {
		part.path = pullrequestmarks.Path(startup.StateDirectory)
	}
	return part
}

// startupNotice is the command that tells the user, once, that the marks file
// could not be used.
func (part *marksPart) startupNotice() tea.Cmd {
	if part.unreadable == nil {
		return nil
	}
	return Error(fmt.Errorf("%w; marks are not saved this session", part.unreadable))
}

// update applies a marks message. broadcast sends a message to every view.
func (part *marksPart) update(message tea.Msg, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	switch message := message.(type) {
	case SetMarkMessage:
		return part.set(message, broadcast)
	case ClearMarkMessage:
		return part.clear(message, broadcast)
	case marksSavedMessage:
		return part.saved(message)
	}
	return nil
}

func (part *marksPart) set(message SetMarkMessage, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	changed, err := part.marks.Set(message.Key, message.Text)
	if err != nil {
		return Error(fmt.Errorf("not marked: %w", describeMarkError(err)))
	}
	part.marks = changed
	text, _ := changed.Mark(message.Key)
	return part.changed(fmt.Sprintf("Marked #%d", message.Key.Number), ": "+terminaltext.Clean(text), broadcast)
}

func (part *marksPart) clear(message ClearMarkMessage, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	part.marks = part.marks.Clear(message.Key)
	return part.changed(fmt.Sprintf("Unmarked #%d", message.Key.Number), "", broadcast)
}

// changed tells the views, starts or queues the save and reports done in the
// status bar. The report is an error when the change can't be saved, and then
// leaves the mark text out: the root logs every error it shows, and mark text
// never goes in the log. detail is the part of the report that holds it.
func (part *marksPart) changed(done, detail string, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	told := broadcast(MarksMessage{Marks: part.marks})
	switch {
	case part.path == "":
		// Session only: nothing to save and nothing to say.
	case part.unreadable != nil:
		return tea.Batch(told, Error(errors.New(done+" (not saved: the marks file could not be read at startup)")))
	case part.saving:
		part.pending = true
	default:
		return tea.Batch(told, Info(done+detail), part.save(done))
	}
	return tea.Batch(told, Info(done+detail))
}

// save starts a save of the current marks; the caller has checked that none is
// in flight. An encoding failure is reported against done.
func (part *marksPart) save(done string) tea.Cmd {
	data, err := pullrequestmarks.Encode(part.marks)
	if err != nil {
		return Error(fmt.Errorf("%s (not saved: %w)", done, err))
	}
	part.saving = true
	path := part.path
	return func() tea.Msg { return marksSavedMessage{err: pullrequestmarks.Write(path, data)} }
}

// saved ends the save in flight, reports a failure and starts the save that
// changes made meanwhile asked for.
func (part *marksPart) saved(message marksSavedMessage) tea.Cmd {
	part.saving = false
	var failure tea.Cmd
	if message.err != nil {
		failure = Error(fmt.Errorf("marks not saved: %w", message.err))
	}
	if !part.pending {
		return failure
	}
	part.pending = false
	return tea.Batch(failure, part.save("Marks changed"))
}

// describeMarkError adds the limit to the error for a mark that is too long.
func describeMarkError(err error) error {
	if errors.Is(err, pullrequestmarks.ErrTooLong) {
		return fmt.Errorf("%w (at most %d characters)", err, pullrequestmarks.MaxLength)
	}
	return err
}

// ClientHost is the host the client's rows come from, or "" when it doesn't
// say. It is never guessed, so a mark is never filed under the wrong host.
func ClientHost(client github.Client) string {
	if identity, ok := client.(interface{ Host() string }); ok {
		return identity.Host()
	}
	return ""
}

// MarkKey is the key of a pull request's mark: the host its rows were fetched
// from, and the pull request's repository and number.
func MarkKey(host string, pullRequest github.PullRequest) pullrequestmarks.Key {
	return pullrequestmarks.Key{Host: host, Repository: pullRequest.Repository, Number: pullRequest.Number}
}

// MarkCommand is `:mark text` in a list, for the pull request under the cursor;
// found is false when there is none. The refusals are shown here; the root
// checks the text itself.
func MarkCommand(host string, pullRequest github.PullRequest, found bool, text string) tea.Cmd {
	switch {
	case text == "":
		return Error(errors.New(":mark needs a name"))
	case !found:
		return Error(errors.New("no pull request to mark"))
	case host == "":
		return Error(errors.New("not marked: the host of this list is unknown"))
	}
	key := MarkKey(host, pullRequest)
	return messageCommand(SetMarkMessage{Key: key, Text: text})
}

// UnmarkCommand is `:unmark` in a list, for the pull request under the cursor;
// found is false when there is none. marks are the last ones the list was sent.
func UnmarkCommand(host string, marks pullrequestmarks.Marks, pullRequest github.PullRequest, found bool) tea.Cmd {
	if !found {
		return Error(errors.New("no pull request to unmark"))
	}
	if host == "" {
		return Error(errors.New("not unmarked: the host of this list is unknown"))
	}
	key := MarkKey(host, pullRequest)
	if _, marked := marks.Mark(key); !marked {
		return Error(fmt.Errorf("#%d has no mark", pullRequest.Number))
	}
	return messageCommand(ClearMarkMessage{Key: key})
}
