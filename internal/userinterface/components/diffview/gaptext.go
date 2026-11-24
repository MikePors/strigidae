package diffview

import "strings"

// HasFileText reports whether SetFileText has given the text of the shown
// file, which ExpandGap needs.
func (model *Model) HasFileText() bool { return model.fileLines != nil }

// SetFileText gives the text of the shown file at the diff's head, so gaps can
// be expanded and the lines below the last hunk counted. It fails, and keeps
// the gaps closed, for a binary file or one over MaximumFileSize. Text given
// for a file that has no gaps is ignored. Another file, or a refresh of this
// one, forgets it.
func (model *Model) SetFileText(text string) error {
	if !canExpand(model.file) {
		return nil
	}
	switch {
	case len(text) > MaximumFileSize:
		return ErrFileTooLarge
	case strings.IndexByte(text, 0) >= 0:
		return ErrFileBinary
	}
	if model.fileLines != nil {
		return nil // already read: a second answer to a repeated request changes nothing
	}
	model.fileLines = splitLines(text)
	model.rebuild(0)
	return nil
}

// splitLines is the lines of text without their line endings.
func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the newline that ends the last line
	}
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}
	return lines
}
