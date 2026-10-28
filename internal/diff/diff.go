// Package diff parses the multi-file unified diff that GitHub returns (git's
// extended format) into files, hunks and lines. It is pure: no UI, no I/O.
//
// Parse never panics. On malformed or truncated input it returns everything it
// could recover together with a *ParseError describing the first problem, so
// callers may choose to show the partial result.
package diff

import "strconv"

// Status says how a file changed.
type Status int

// File statuses.
const (
	Modified Status = iota
	Added
	Deleted
	Renamed
	Copied
)

// String returns a lower-case name for the status.
func (status Status) String() string {
	switch status {
	case Modified:
		return "modified"
	case Added:
		return "added"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	case Copied:
		return "copied"
	}
	return "unknown"
}

// LineKind classifies a hunk line.
type LineKind int

// Line kinds.
const (
	Context LineKind = iota
	Add
	Delete
	// NoNewline is the "\ No newline at end of file" marker. It belongs to the
	// line directly above it and has no line numbers.
	NoNewline
)

// String returns a lower-case name for the kind.
func (kind LineKind) String() string {
	switch kind {
	case Context:
		return "context"
	case Add:
		return "add"
	case Delete:
		return "delete"
	case NoNewline:
		return "no-newline"
	}
	return "unknown"
}

// Line is one line of a hunk.
type Line struct {
	Kind LineKind
	// Old and New are 1-based line numbers; 0 when the line does not exist on
	// that side (New for deletions, Old for additions, both for markers).
	Old, New int
	// Text is the content without the leading ' ', '+' or '-' and without a
	// trailing carriage return (CRLF files parse like LF files). For a
	// NoNewline marker it is the marker's message.
	Text string
}

// Hunk is one "@@ -a,b +c,d @@ section" block.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	// Section is the text after the closing "@@" (usually the enclosing
	// function), without surrounding space.
	Section string
	Lines   []Line
}

// Header reconstructs the hunk header line.
func (hunk Hunk) Header() string {
	hunkString := "@@ -" + rangeText(hunk.OldStart, hunk.OldLines) + " +" + rangeText(hunk.NewStart, hunk.NewLines) + " @@"
	if hunk.Section != "" {
		hunkString += " " + hunk.Section
	}
	return hunkString
}

func rangeText(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(count)
}

// File is the diff of a single file.
type File struct {
	// OldPath is empty for added files; NewPath is empty for deleted files.
	OldPath, NewPath string
	Status           Status
	Binary           bool
	// OldMode and NewMode are git file modes such as "100644"; empty when the
	// diff does not mention them. A mode change has both set and different.
	OldMode, NewMode string
	// Similarity is the rename/copy similarity percentage (0 when absent).
	Similarity int
	Hunks      []Hunk
	// PatchMissing is set on a diff built file by file when GitHub gave this
	// file no patch: it is binary, or too large on its own.
	PatchMissing bool
}

// Path returns the new path, falling back to the old one (deleted files).
func (file *File) Path() string {
	if file.NewPath != "" {
		return file.NewPath
	}
	return file.OldPath
}

// ModeChanged reports whether the file's mode changed.
func (file *File) ModeChanged() bool {
	return file.OldMode != "" && file.NewMode != "" && file.OldMode != file.NewMode
}

// Diff is a whole multi-file diff.
type Diff struct {
	Files []*File
}

// File looks a file up by path, matching the new path first and then the old
// path (so a renamed file is found under either name). It returns nil when
// there is no match.
func (diff *Diff) File(path string) *File {
	if path == "" {
		return nil
	}
	for _, file := range diff.Files {
		if file.NewPath == path {
			return file
		}
	}
	for _, file := range diff.Files {
		if file.OldPath == path {
			return file
		}
	}
	return nil
}
