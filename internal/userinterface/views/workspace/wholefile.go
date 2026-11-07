package workspace

import (
	"context"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

// BlobReader reads a file's content by the blob object identifier on the
// diff's index line, for pull request number. *gitrepository.BlobReader reads
// it from the local checkout. It is called inside a tea.Cmd.
type BlobReader interface {
	ReadBlob(ctx context.Context, number int, blobObjectIdentifier string) ([]byte, error)
}

// maximumWholeFiles bounds how many sides of files WholeFiles keeps; when it
// is full it starts again empty.
const maximumWholeFiles = 200

// WholeFiles colours a modified, renamed or copied file from its whole text
// on each side rather than hunk by hunk, so a hunk that starts inside a
// comment or string is coloured right. It holds the reader and the spans of
// every side read so far, by path and blob, so they last as long as the
// value: a view that opens workspaces keeps one for the session. It is used
// on the UI goroutine only; the reading and lexing happen in a tea.Cmd.
type WholeFiles struct {
	reader BlobReader
	spans  map[wholeFileKey][][]diffview.Span // nil: the side could not be used
}

// NewWholeFiles returns an empty cache reading with reader.
func NewWholeFiles(reader BlobReader) *WholeFiles {
	return &WholeFiles{reader: reader, spans: map[wholeFileKey][][]diffview.Span{}}
}

// WithWholeFiles makes the workspace colour files from their whole text (see
// WholeFiles). Without it every file is coloured hunk by hunk.
func WithWholeFiles(files *WholeFiles) Option {
	return func(view *View) { view.wholeFiles = files }
}

// wholeFileKey is one side of a file. The path is part of it because the
// path picks the language.
type wholeFileKey struct {
	path             string
	objectIdentifier string
}

// wholeFileMessage is the result of reading and lexing one side.
type wholeFileMessage struct {
	owner *View
	key   wholeFileKey
	lines [][]diffview.Span
	err   error
}

// wholeFileSides returns the commands that read the sides of the shown file
// not read yet, and gives the pane the sides already read. It runs after
// every update, so it is cheap when there is nothing to do.
func (view *View) wholeFileSides() tea.Cmd {
	if view.wholeFiles == nil || view.mode != contentFile || view.diff.Parsed == nil {
		return nil
	}
	file := view.diff.Parsed.File(view.paneFile.Path)
	if file == nil || file.Binary || len(file.Hunks) == 0 {
		return nil
	}
	// An added or deleted file's diff holds the whole file already.
	if file.Status != diff.Modified && file.Status != diff.Renamed && file.Status != diff.Copied {
		return nil
	}
	sides := []struct {
		side       github.DiffSide
		identifier string
	}{{github.SideLeft, file.OldObjectIdentifier}, {github.SideRight, file.NewObjectIdentifier}}
	var commands []tea.Cmd
	for _, side := range sides {
		if side.identifier == "" {
			continue
		}
		key := wholeFileKey{path: file.Path(), objectIdentifier: side.identifier}
		if lines, ok := view.wholeFiles.spans[key]; ok {
			if lines != nil {
				view.filePane.SetFileSpans(side.side, side.identifier, lines)
			}
			continue
		}
		if view.wholeFilePending[key] {
			continue
		}
		if view.wholeFilePending == nil {
			view.wholeFilePending = map[wholeFileKey]bool{}
		}
		view.wholeFilePending[key] = true
		commands = append(commands, view.readWholeFile(key))
	}
	return tea.Batch(commands...)
}

// readWholeFile reads one side and lexes it, off the UI goroutine.
func (view *View) readWholeFile(key wholeFileKey) tea.Cmd {
	reader, number, timeouts, owner := view.wholeFiles.reader, view.number, view.timeouts, view
	return func() tea.Msg {
		// The deadline covers fetching the pull request's head too.
		ctx, cancel := timeouts.LoadContext()
		defer cancel()
		contents, err := reader.ReadBlob(ctx, number, key.objectIdentifier)
		if err != nil {
			return wholeFileMessage{owner: owner, key: key, err: err}
		}
		lines, err := diffview.HighlightFile(key.path, contents)
		return wholeFileMessage{owner: owner, key: key, lines: lines, err: err}
	}
}

// wholeFileRead keeps the result of reading one side. The pane is given it
// after the update if the file is still shown (wholeFileSides); otherwise it
// waits in the cache.
func (view *View) wholeFileRead(message wholeFileMessage) {
	delete(view.wholeFilePending, message.key)
	if message.err != nil {
		// The path is left out: it is the pull request author's text.
		slog.Info("colour a file hunk by hunk: whole file unavailable", "blob", message.key.objectIdentifier, "err", message.err)
	}
	if len(view.wholeFiles.spans) >= maximumWholeFiles {
		view.wholeFiles.spans = map[wholeFileKey][][]diffview.Span{}
	}
	view.wholeFiles.spans[message.key] = message.lines
}
