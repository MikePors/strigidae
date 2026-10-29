package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// HunkMarks remembers which hunks the user marked reviewed. The marks are
// local to strig (GitHub has no hunk-level state) and last as long as the
// value: a view that opens workspaces keeps one, so marks survive a refresh
// and reopening the pull request until strig quits.
//
// A hunk is identified by its content, not its position, so a push that
// edits other hunks leaves its mark alone, and a push that edits it clears
// it.
type HunkMarks struct {
	reviewed map[hunkKey]bool
	// autoViewed holds the files strig marked viewed because all their hunks
	// were reviewed (the autoviewed option). Only these are unmarked again
	// when a hunk is unmarked; a file the user marked with v never is.
	autoViewed map[fileKey]bool
}

// NewHunkMarks returns an empty set of hunk marks.
func NewHunkMarks() *HunkMarks {
	return &HunkMarks{reviewed: map[hunkKey]bool{}, autoViewed: map[fileKey]bool{}}
}

// fileKey identifies one file of one pull request.
type fileKey struct {
	repository github.Repository
	number     int
	path       string
}

// hunkKey identifies one hunk of one pull request.
type hunkKey struct {
	repository  github.Repository
	number      int
	path        string
	fingerprint string
}

// HunkMarks returns the marks this workspace records into.
func (view *View) HunkMarks() *HunkMarks { return view.hunkMarks }

// WithHunkMarks shares marks with other workspaces, so they outlive this one.
// Without it the workspace keeps its own, lost when it closes.
func WithHunkMarks(marks *HunkMarks) Option {
	return func(view *View) { view.hunkMarks = marks }
}

// fingerprintCache holds the fingerprints of one file's hunks: rendering asks
// for them on every header row.
type fingerprintCache struct {
	file         *diff.File
	fingerprints []string
	computed     int // how many files were hashed, for tests
}

// fingerprintsOf returns a content fingerprint for each hunk of file: a
// SHA-256 of its lines (kind and text, not line numbers), plus how many
// earlier hunks in the file share that hash, so identical hunks keep separate
// marks. The hash is cryptographic because the content is the pull request
// author's: a push must not be able to keep a mark on changed code by
// colliding with the reviewed version.
func (cache *fingerprintCache) fingerprintsOf(file *diff.File) []string {
	if cache.file == file {
		return cache.fingerprints
	}
	cache.computed++
	seen := map[string]int{}
	fingerprints := make([]string, len(file.Hunks))
	for i, hunk := range file.Hunks {
		hasher := sha256.New()
		for _, line := range hunk.Lines {
			_, _ = fmt.Fprintf(hasher, "%d %s\n", line.Kind, line.Text)
		}
		sum := hex.EncodeToString(hasher.Sum(nil))
		fingerprints[i] = sum + "/" + strconv.Itoa(seen[sum])
		seen[sum]++
	}
	cache.file, cache.fingerprints = file, fingerprints
	return fingerprints
}

func (view *View) hunkKey(file *diff.File, hunk int) hunkKey {
	return hunkKey{view.repository, view.number, file.Path(), view.hunkFingerprints.fingerprintsOf(file)[hunk]}
}

// hunkReviewed reports whether the user marked a hunk of file reviewed.
func (view *View) hunkReviewed(file *diff.File, hunk int) bool {
	return view.hunkMarks.reviewed[view.hunkKey(file, hunk)]
}

// reviewedHunks counts the reviewed hunks of file.
func (view *View) reviewedHunks(file *diff.File) int {
	count := 0
	for hunk := range file.Hunks {
		if view.hunkReviewed(file, hunk) {
			count++
		}
	}
	return count
}

// toggleHunkReviewed flips the reviewed mark of the hunk at the diff cursor.
// Nothing is sent to GitHub.
func (view *View) toggleHunkReviewed() tea.Cmd {
	if hidden := view.diffHidden("mark the hunk"); hidden != nil {
		return hidden
	}
	context := view.filePane.Context()
	file, hunk := context.File, context.Hunk
	if !context.InHunk || view.diff.Parsed == nil || file != view.diff.Parsed.File(view.selectedPath()) {
		return userinterface.Info("no hunk at the cursor")
	}
	key := view.hunkKey(file, hunk)
	reviewed := !view.hunkMarks.reviewed[key]
	state := "reviewed"
	if reviewed {
		view.hunkMarks.reviewed[key] = true
	} else {
		delete(view.hunkMarks.reviewed, key)
		state = "not reviewed"
	}
	// Said out loud: the mark alone is easy to miss.
	fade := view.hunkMarkChanged(file, hunk, reviewed)
	command, text := view.followHunks(file, reviewed)
	switch {
	case text != "":
		return tea.Batch(fade, command, userinterface.Info(text))
	case command != nil:
		// Only the error: a batched info would race it for the status bar.
		return tea.Batch(fade, command)
	}
	return tea.Batch(fade, userinterface.Info(fmt.Sprintf("hunk %d/%d %s", hunk+1, len(file.Hunks), state)))
}

// followHunks keeps the selected file's viewed mark in step with its hunks
// when the autoviewed option is on: reviewing the last hunk marks the file
// viewed, as v would, and unreviewing a hunk of a file strig marked that way
// unmarks it again. It returns the mutation and what to say, or "" when the
// file's mark stays as it is (or can't be sent, when the command is the
// error to show).
func (view *View) followHunks(file *diff.File, reviewed bool) (tea.Cmd, string) {
	selected := view.selection()
	if !view.options.AutoViewed() || selected == 0 || selected > len(view.files()) {
		return nil, ""
	}
	changed := &view.detail.Files[selected-1]
	key := fileKey{view.repository, view.number, changed.Path}
	path := terminaltext.Clean(changed.Path)
	switch {
	case reviewed && view.reviewedHunks(file) == len(file.Hunks) && changed.Viewed != github.FileViewed:
		command := view.setFileViewed(changed, github.FileViewed)
		if view.detail.Identifier == "" {
			return command, "" // the error saying why; nothing was marked
		}
		view.hunkMarks.autoViewed[key] = true
		return command, "Marked " + path + " viewed (all hunks reviewed)"
	case !reviewed && view.hunkMarks.autoViewed[key]:
		delete(view.hunkMarks.autoViewed, key)
		if changed.Viewed != github.FileViewed {
			return nil, ""
		}
		return view.setFileViewed(changed, github.FileUnviewed), "Marked " + path + " not viewed (a hunk is no longer reviewed)"
	}
	return nil, ""
}

// hunkProgress is the diff pane title's suffix once any hunk of the selected
// file is marked reviewed, e.g. " · 2/5 hunks reviewed".
func (view *View) hunkProgress() string {
	path := view.selectedPath()
	if path == "" || view.diff.Parsed == nil {
		return ""
	}
	file := view.diff.Parsed.File(path)
	if file == nil {
		return ""
	}
	reviewed := view.reviewedHunks(file)
	if reviewed == 0 {
		return ""
	}
	return fmt.Sprintf(" · %d/%d hunks reviewed", reviewed, len(file.Hunks))
}
