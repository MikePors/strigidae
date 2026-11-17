package userinterface

import (
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/pullrequestnotes"
)

// NotesMessage gives views every note, after any of them changed and to each
// view as it is pushed. A view keeps the last one it was sent and derives what
// it draws from it. Notes are a value, so nothing is shared with the root.
type NotesMessage struct{ Notes pullrequestnotes.Notes }

// SetNoteMessage asks the root to give a pull request a note, replacing any it
// had. The workspace sends it when `:note` returns from the editor.
type SetNoteMessage struct {
	Key  pullrequestnotes.Key
	Text string
}

// ClearNoteMessage asks the root to remove a pull request's note. The workspace
// sends it after the `y/N` of `:note-delete`.
type ClearNoteMessage struct{ Key pullrequestnotes.Key }

// notesSavedMessage is the result of the save in flight.
type notesSavedMessage struct{ err error }

// notesPart is the root's slice of state for pull request notes: the notes, the
// file they are saved in, and the bookkeeping of saves. It has the shape of
// marksPart, and the same rules: at most one save in flight, and a change made
// meanwhile is saved when that one ends, so an older copy never lands after a
// newer one.
type notesPart struct {
	notes pullrequestnotes.Notes
	// path is the file the notes are saved in; empty keeps them for the
	// session only.
	path string
	// unreadable is why the file could not be read at startup. The file is
	// then never written, so it is not overwritten before the user has looked.
	unreadable error
	saving     bool
	pending    bool
	// saver writes the file for the saves and for the flush at quit; it orders
	// them by generation, which grows with every change.
	saver      *pullrequestnotes.Saver
	generation int
}

func newNotesPart(startup Options) notesPart {
	part := notesPart{notes: startup.Notes, unreadable: startup.NotesError}
	if startup.StateDirectory != "" {
		part.path = pullrequestnotes.Path(startup.StateDirectory)
		part.saver = pullrequestnotes.NewSaver(part.path)
	}
	return part
}

// startupNotice is the command that tells the user, once, that the notes file
// could not be used.
func (part *notesPart) startupNotice() tea.Cmd {
	if part.unreadable == nil {
		return nil
	}
	return Error(fmt.Errorf("%w; notes are not saved this session", part.unreadable))
}

// update applies a notes message. broadcast sends a message to every view.
func (part *notesPart) update(message tea.Msg, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	switch message := message.(type) {
	case SetNoteMessage:
		return part.set(message, broadcast)
	case ClearNoteMessage:
		part.notes = part.notes.Clear(message.Key)
		return part.changed(fmt.Sprintf("Deleted the note on #%d", message.Key.Number), broadcast)
	case notesSavedMessage:
		return part.saved(message)
	}
	return nil
}

func (part *notesPart) set(message SetNoteMessage, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	changed, err := part.notes.Set(message.Key, message.Text)
	if err != nil {
		return Error(fmt.Errorf("note not saved: %w", err))
	}
	part.notes = changed
	return part.changed(fmt.Sprintf("Saved the note on #%d", message.Key.Number), broadcast)
}

// changed tells the views, starts or queues the save and reports done in the
// status bar. The report never holds the note: the root logs every error it
// shows, and note text never goes in the log.
func (part *notesPart) changed(done string, broadcast func(tea.Msg) tea.Cmd) tea.Cmd {
	part.generation++
	told := broadcast(NotesMessage{Notes: part.notes})
	switch {
	case part.path == "":
		// Session only: nothing to save and nothing to say.
	case part.unreadable != nil:
		return tea.Batch(told, Error(errors.New(done+" (not kept: the notes file could not be read at startup)")))
	case part.saving:
		part.pending = true
	default:
		return tea.Batch(told, Info(done), part.save(done))
	}
	return tea.Batch(told, Info(done))
}

// save starts a save of the current notes; the caller has checked that none is
// in flight. An encoding failure is reported against done.
func (part *notesPart) save(done string) tea.Cmd {
	data, err := pullrequestnotes.Encode(part.notes)
	if err != nil {
		return Error(fmt.Errorf("%s (not kept: %w)", done, err))
	}
	part.saving = true
	saver, generation := part.saver, part.generation
	return func() tea.Msg { return notesSavedMessage{err: saver.Save(generation, data)} }
}

// flush writes the latest notes at quit, in a goroutine, and closes the channel
// it returns when that is done (at once when there is nothing to write). A save
// still running, or queued behind one, is not waited for: the saver skips the
// older copy. The error, which never holds a note, goes to the log.
func (part *notesPart) flush(log *slog.Logger) <-chan struct{} {
	finished := make(chan struct{})
	if part.saver == nil || part.unreadable != nil || part.generation == 0 {
		close(finished)
		return finished
	}
	data, err := pullrequestnotes.Encode(part.notes)
	if err != nil {
		log.Warn("notes not saved at quit", "err", err)
		close(finished)
		return finished
	}
	saver, generation := part.saver, part.generation
	go func() {
		defer close(finished)
		if err := saver.Save(generation, data); err != nil {
			log.Warn("notes not saved at quit", "err", err)
		}
	}()
	return finished
}

// saved ends the save in flight, reports a failure and starts the save that
// changes made meanwhile asked for.
func (part *notesPart) saved(message notesSavedMessage) tea.Cmd {
	part.saving = false
	var failure tea.Cmd
	if message.err != nil {
		failure = Error(fmt.Errorf("notes not saved: %w", message.err))
	}
	if !part.pending {
		return failure
	}
	part.pending = false
	return tea.Batch(failure, part.save("Notes changed"))
}

// SetNoteCommand is the command that gives a pull request a note. A client with
// no host can't say where the pull request lives, so nothing is kept for it.
func SetNoteCommand(key pullrequestnotes.Key, text string) tea.Cmd {
	if key.Host == "" {
		return Error(errors.New("note not saved: the host of this pull request is unknown"))
	}
	return messageCommand(SetNoteMessage{Key: key, Text: text})
}

// ClearNoteCommand is the command that removes a pull request's note, after the
// caller's `y/N`.
func ClearNoteCommand(key pullrequestnotes.Key) tea.Cmd {
	return messageCommand(ClearNoteMessage{Key: key})
}
