package workspace

import (
	"fmt"
	"strings"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
)

// perFileNotice is shown once a diff GitHub refused whole has been built from
// the pull request's file list.
const perFileNotice = "large pull request: diff loaded file by file"

// perFileDiff builds the diff of a pull request too large for GitHub to diff
// whole, each file from its own patch in the file list. raw stands in for
// the unified diff text as DiffState.Raw: it changes whenever a file or patch
// does. A file GitHub gave no patch is marked PatchMissing, unless it is a
// rename or copy with no changed lines, which has nothing to patch.
func perFileDiff(patches []github.FilePatch) (raw string, built *diff.Diff, err error) {
	var text strings.Builder
	built = &diff.Diff{}
	for _, patch := range patches {
		file := &diff.File{OldPath: patch.Path, NewPath: patch.Path}
		switch patch.Change {
		case github.FileAdded:
			file.Status, file.OldPath = diff.Added, ""
		case github.FileDeleted:
			file.Status, file.NewPath = diff.Deleted, ""
		case github.FileRenamed, github.FileCopied:
			file.Status = diff.Renamed
			if patch.Change == github.FileCopied {
				file.Status = diff.Copied
			}
			if patch.PreviousPath != "" {
				file.OldPath = patch.PreviousPath
			}
		}
		fmt.Fprintf(&text, "%s\x00%s\x00%s\x00%d\x00%d\x00%s\n", patch.Change, patch.PreviousPath, patch.Path, patch.Additions, patch.Deletions, patch.Patch)
		if patch.Patch == "" {
			pureMove := (file.Status == diff.Renamed || file.Status == diff.Copied) && patch.Additions+patch.Deletions == 0
			file.PatchMissing = !pureMove
		} else {
			hunks, parseError := diff.ParseHunks(patch.Patch)
			file.Hunks = hunks
			if parseError != nil && err == nil {
				err = fmt.Errorf("patch of %s: %w", patch.Path, parseError)
			}
		}
		built.Files = append(built.Files, file)
	}
	return text.String(), built, err
}
