package workspace

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/pullrequestnotes"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// :note keeps a private note on the pull request. It is local to strig: nothing
// is sent to GitHub, so there is no y/N for writing it and no check against
// GitHub's state. The root owns the notes and saves them; the view only asks
// for a change and draws the note on the Overview. Deleting a note asks y/N, as
// every deletion does.

// noteTextMessage is what the editor returned for `:note`.
type noteTextMessage struct {
	origin origin
	text   string
	err    error
}

// noteKey is the key of this pull request's note.
func (view *View) noteKey() pullrequestnotes.Key {
	return pullrequestnotes.Key{Host: view.host(), Repository: view.repository, Number: view.number}
}

// note is this pull request's note, or "" when it has none.
func (view *View) note() string {
	text, _ := view.notes.Note(view.noteKey())
	return text
}

// editNote opens the editor on the text of an edit that could not be kept, or
// else on the note, empty when there is none.
func (view *View) editNote() tea.Cmd {
	origin := view.origin()
	initial := view.noteDraft
	if initial == "" {
		initial = view.note()
	}
	return view.edit(initial, func(text string, err error) tea.Msg {
		return noteTextMessage{origin: origin, text: text, err: err}
	})
}

// noteEdited sends what the editor returned to the root, after checking it
// here, while the text can still be kept: the editor's file is gone by now, so
// text that can't be saved becomes the draft the next `:note` opens on. An empty
// note is an error and not a deletion, so a failed editor can't lose the note.
func (view *View) noteEdited(message noteTextMessage) tea.Cmd {
	text := pullrequestnotes.Clean(message.text)
	if message.err != nil {
		return view.noteKept(text, terminaltext.SafeError(fmt.Errorf("edit note: %w", message.err)))
	}
	if text == "" {
		return userinterface.Error(errors.New("empty note, nothing saved (:note-delete removes it)"))
	}
	if err := pullrequestnotes.Validate(text); err != nil {
		return view.noteKept(text, fmt.Errorf("note not saved: %w", err))
	}
	key := view.noteKey()
	if key.Host == "" {
		return view.noteKept(text, errors.New("note not saved: the host of this pull request is unknown"))
	}
	view.noteDraft = ""
	if text == view.note() {
		return userinterface.Info(noticeNoChanges)
	}
	return userinterface.SetNoteCommand(key, text)
}

// noteKept remembers text that could not be saved and shows why. An empty text
// leaves the draft as it was.
func (view *View) noteKept(text string, reason error) tea.Cmd {
	if text == "" {
		return userinterface.Error(reason)
	}
	view.noteDraft = text
	return userinterface.Error(fmt.Errorf("%w (your text is kept: :note opens on it again)", reason))
}

// deleteNote asks before removing the note.
func (view *View) deleteNote() tea.Cmd {
	if view.note() == "" {
		return userinterface.Info(fmt.Sprintf("#%d has no note", view.number))
	}
	return userinterface.Confirm(fmt.Sprintf("Delete the private note on #%d?", view.number), userinterface.ClearNoteCommand(view.noteKey()))
}
