package workspace

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// fileTextWantedMessage is how the file pane asks for the text of its file,
// which it needs to open a gap of unchanged lines (see diffpane.go). It
// travels through the program like any result, so it names the pane that
// asked.
type fileTextWantedMessage struct {
	pane FilePane
	path string
}

// fileTextKey names a file's text: the path at the head commit the diff on
// screen was loaded for.
type fileTextKey struct{ path, head string }

// fileTextMessage is the result of reading one file's text.
type fileTextMessage struct {
	owner *View
	key   fileTextKey
	text  string
	err   error
}

// fileTextWanted starts reading the text of the file the pane asked for, at
// the head of the diff on screen. The text is not kept here: the pane holds it
// as long as the file is shown, and asks again for another file.
func (view *View) fileTextWanted(message fileTextWantedMessage) tea.Cmd {
	if message.pane != view.filePane || message.path != view.paneFile.Path {
		return nil // not the file shown any more
	}
	if view.filePane.HasFileText(message.path) {
		return nil // a request made before the text arrived, such as a second press of enter
	}
	if view.diffHead == "" {
		return userinterface.Info("can't show more lines: the head commit of the diff is not known")
	}
	key := fileTextKey{path: message.path, head: view.diffHead}
	if view.fileTextPending[key] {
		return nil // already on its way
	}
	if view.fileTextPending == nil {
		view.fileTextPending = map[fileTextKey]bool{}
	}
	view.fileTextPending[key] = true
	client, repository, timeouts, owner := view.client, view.repository, view.timeouts, view
	return func() tea.Msg {
		ctx, cancel := timeouts.LoadContext()
		defer cancel()
		text, err := client.FileText(ctx, repository, key.head, key.path)
		return fileTextMessage{owner: owner, key: key, text: text, err: err}
	}
}

// fileTextRead gives the pane the text it asked for, if the file and the diff
// are still the ones it was read for.
func (view *View) fileTextRead(message fileTextMessage) tea.Cmd {
	delete(view.fileTextPending, message.key)
	if message.key.path != view.paneFile.Path || message.key.head != view.diffHead {
		return nil
	}
	if message.err != nil && view.mode != contentFile {
		return nil // the reader has left the diff: nothing to say about it now
	}
	switch {
	case errors.Is(message.err, github.ErrNotFound), errors.Is(message.err, github.ErrNoFileText):
		return userinterface.Info("can't show more lines: GitHub has no text for this file")
	case message.err != nil:
		return userinterface.Error(terminaltext.SafeError(fmt.Errorf("read the file's lines: %w", message.err)))
	}
	return view.filePane.SetFileText(message.key.path, message.text)
}
