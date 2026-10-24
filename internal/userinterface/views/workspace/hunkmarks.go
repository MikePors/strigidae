package workspace

import (
	"fmt"
	"hash/fnv"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
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
}

// NewHunkMarks returns an empty set of hunk marks.
func NewHunkMarks() *HunkMarks { return &HunkMarks{reviewed: map[hunkKey]bool{}} }

// hunkKey identifies one hunk of one pull request.
type hunkKey struct {
	repository  github.Repository
	number      int
	path        string
	fingerprint string
}

// WithHunkMarks shares marks with other workspaces, so they outlive this one.
// Without it the workspace keeps its own, lost when it closes.
func WithHunkMarks(marks *HunkMarks) Option {
	return func(view *View) { view.hunkMarks = marks }
}

// hunkFingerprints caches the fingerprints of one file's hunks: rendering
// asks for them on every header row.
type hunkFingerprints struct {
	file         *diff.File
	fingerprints []string
}

// fingerprintsOf returns a content fingerprint for each hunk of file: a hash
// of its lines (kind and text, not line numbers), plus how many earlier
// hunks in the file share that hash, so identical hunks keep separate marks.
func (cache *hunkFingerprints) fingerprintsOf(file *diff.File) []string {
	if cache.file == file {
		return cache.fingerprints
	}
	seen := map[uint64]int{}
	fingerprints := make([]string, len(file.Hunks))
	for i, hunk := range file.Hunks {
		hasher := fnv.New64a()
		for _, line := range hunk.Lines {
			_, _ = fmt.Fprintf(hasher, "%d %s\n", line.Kind, line.Text)
		}
		sum := hasher.Sum64()
		fingerprints[i] = strconv.FormatUint(sum, 16) + "/" + strconv.Itoa(seen[sum])
		seen[sum]++
	}
	cache.file, cache.fingerprints = file, fingerprints
	return fingerprints
}

func (view *View) hunkKey(file *diff.File, hunk int) hunkKey {
	return hunkKey{view.repository, view.number, file.Path(), view.hunkPrints.fingerprintsOf(file)[hunk]}
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
	file, hunk, ok := view.filePane.CursorHunk()
	if !ok || view.diff.Parsed == nil || file != view.diff.Parsed.File(view.selectedPath()) {
		return userinterface.Info("no hunk at the cursor")
	}
	key := view.hunkKey(file, hunk)
	if view.hunkMarks.reviewed[key] {
		delete(view.hunkMarks.reviewed, key)
	} else {
		view.hunkMarks.reviewed[key] = true
	}
	return nil
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
