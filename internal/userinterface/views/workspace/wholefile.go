package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

// BlobReader reads a file's content by the blob object identifier on the
// diff's index line, for pull request number. *gitrepository.BlobReader reads
// it from the local checkout. It is called inside a tea.Cmd.
type BlobReader interface {
	ReadBlob(ctx context.Context, number int, blobObjectIdentifier string) ([]byte, error)
}

// maximumWholeFileBytes bounds the memory WholeFiles keeps, as estimated by
// spansSize; when a side would pass it the cache starts again empty. A 1 MiB
// side (diffview.MaximumFileSize) of short tokens costs some tens of
// megabytes, so this keeps a few of the largest files or many ordinary ones.
const maximumWholeFileBytes = 256 << 20

// spanOverhead is the estimated cost of one span besides its text: the
// string header, the token type and the slice slot.
const spanOverhead = 32

// WholeFiles colours a modified, renamed or copied file from its whole text
// on each side rather than hunk by hunk, so a hunk that starts inside a
// comment or string is coloured right. It holds the reader and the spans of
// every side read so far, by path and blob, so they last as long as the
// value: a view that opens workspaces keeps one for the session. It is used
// on the UI goroutine only; the reading and lexing happen in a tea.Cmd.
type WholeFiles struct {
	reader BlobReader
	spans  map[wholeFileKey][][]diffview.Span // nil: the side could not be used
	bytes  int                                // estimated size of spans
	limit  int                                // bound on bytes; maximumWholeFileBytes
}

// NewWholeFiles returns an empty cache. Sides are read with reader, the local
// checkout, where it can supply them, and with the GitHub API otherwise; a nil
// reader means the API only.
func NewWholeFiles(reader BlobReader) *WholeFiles {
	return &WholeFiles{reader: reader, spans: map[wholeFileKey][][]diffview.Span{}, limit: maximumWholeFileBytes}
}

// WithWholeFiles makes the workspace colour files from their whole text (see
// WholeFiles). Without it every file is coloured hunk by hunk.
func WithWholeFiles(files *WholeFiles) Option {
	return func(view *View) { view.wholeFiles = files }
}

// WholeFiles returns the cache this workspace reads files whole with, nil
// when it colours files hunk by hunk.
func (view *View) WholeFiles() *WholeFiles { return view.wholeFiles }

// wholeFileKey is one side of a file. The path is part of it because the
// path picks the language.
type wholeFileKey struct {
	path             string
	objectIdentifier string
}

// wholeFileMessage is the result of reading and lexing the sides of one
// file.
type wholeFileMessage struct {
	owner   *View
	results []wholeFileResult
}

// wholeFileResult is one side. checkoutErr is why the checkout could not
// supply it, when it was asked and the API was asked next; source is where
// lines came from.
type wholeFileResult struct {
	key         wholeFileKey
	lines       [][]diffview.Span
	err         error
	source      string
	checkoutErr error
}

// Where a side's text came from.
const (
	sourceCheckout = "checkout"
	sourceAPI      = "api"
)

// wholeFileSides returns the command that reads the sides of the shown file
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
	var unread []wholeFileKey
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
		unread = append(unread, key)
	}
	if len(unread) == 0 {
		return nil
	}
	return view.readWholeFile(unread)
}

// readWholeFile reads the given sides of one file and lexes them, off the UI
// goroutine. The checkout is asked first, when there is one; every side it
// could not supply is asked of the API in one request.
func (view *View) readWholeFile(keys []wholeFileKey) tea.Cmd {
	reader, number, timeouts, owner := view.wholeFiles.reader, view.number, view.timeouts, view
	client, repository := view.client, view.repository
	return func() tea.Msg {
		results := make([]wholeFileResult, len(keys))
		var missing []int
		for i, key := range keys {
			results[i].key = key
			if reader == nil {
				missing = append(missing, i)
				continue
			}
			contents, err := readCheckoutBlob(timeouts, reader, number, key.objectIdentifier)
			if err != nil {
				results[i].checkoutErr = err
				missing = append(missing, i)
				continue
			}
			results[i].source = sourceCheckout
			results[i].lines, results[i].err = diffview.HighlightFile(key.path, contents)
		}
		if len(missing) > 0 {
			readAPIBlobs(timeouts, client, repository, results, missing)
		}
		return wholeFileMessage{owner: owner, results: results}
	}
}

// readCheckoutBlob reads one blob from the checkout under its own deadline,
// which covers fetching the pull request's head too.
func readCheckoutBlob(timeouts userinterface.Timeouts, reader BlobReader, number int, identifier string) ([]byte, error) {
	ctx, cancel := timeouts.LoadContext()
	defer cancel()
	return reader.ReadBlob(ctx, number, identifier)
}

// readAPIBlobs fills in the results at the indexes in missing from one
// BlobTexts request. It uses a fresh deadline: the checkout may have used up
// the first.
func readAPIBlobs(timeouts userinterface.Timeouts, client github.Client, repository github.Repository, results []wholeFileResult, missing []int) {
	ctx, cancel := timeouts.LoadContext()
	defer cancel()
	identifiers := make([]string, len(missing))
	for i, index := range missing {
		identifiers[i] = results[index].key.objectIdentifier
	}
	texts, err := client.BlobTexts(ctx, repository, identifiers)
	if err == nil && len(texts) != len(identifiers) {
		err = fmt.Errorf("asked for %d blobs, got %d", len(identifiers), len(texts))
	}
	for i, index := range missing {
		result := &results[index]
		result.source = sourceAPI
		switch {
		case err != nil:
			result.err = err
		case texts[i].Err != nil:
			result.err = texts[i].Err
		default:
			result.lines, result.err = diffview.HighlightFile(result.key.path, []byte(texts[i].Text))
		}
	}
}

// wholeFileRead keeps the result of reading the sides of a file. The pane is
// given them after the update if the file is still shown (wholeFileSides);
// otherwise they wait in the cache.
//
// A read that only ran out of time is not kept the first time, so the side
// is read once more while the file is shown (it may have waited for another
// read's fetch); a second one is kept as a failure, so a timeout that always
// expires, or a disk always slower than it, cannot start reads without end.
func (view *View) wholeFileRead(message wholeFileMessage) {
	for _, result := range message.results {
		delete(view.wholeFilePending, result.key)
		if result.err == nil {
			slog.Debug("whole file read", "blob", result.key.objectIdentifier, "source", result.source)
		} else {
			// The path is left out: it is the pull request author's text.
			slog.Info("colour a file hunk by hunk: whole file unavailable", "blob", result.key.objectIdentifier, "source", result.source, "checkout_err", result.checkoutErr, "err", result.err)
			timedOut := errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled)
			if timedOut && !view.wholeFileRetried[result.key] {
				if view.wholeFileRetried == nil {
					view.wholeFileRetried = map[wholeFileKey]bool{}
				}
				view.wholeFileRetried[result.key] = true
				continue
			}
		}
		view.wholeFiles.keep(result.key, result.lines)
	}
}

// keep stores one side, starting again empty when it would pass the memory
// bound.
func (files *WholeFiles) keep(key wholeFileKey, lines [][]diffview.Span) {
	size := spansSize(lines)
	if files.bytes+size > files.limit {
		files.spans, files.bytes = map[wholeFileKey][][]diffview.Span{}, 0
	}
	files.spans[key] = lines
	files.bytes += size
}

// spansSize estimates the memory lines hold.
func spansSize(lines [][]diffview.Span) int {
	size := 0
	for _, line := range lines {
		size += spanOverhead
		for _, span := range line {
			size += len(span.Text) + spanOverhead
		}
	}
	return size
}
