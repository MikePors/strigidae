package editor

import (
	"errors"
	"strings"
)

// Errors of ParsePullRequestText, for the caller to tell apart.
var (
	// ErrEmptyTitle: the first line is empty.
	ErrEmptyTitle = errors.New("empty title")
	// ErrMissingBlankLine: the line after the title is not blank.
	ErrMissingBlankLine = errors.New("the title must be followed by a blank line")
)

// FormatPullRequestText is the text a pull request is edited as: the title on
// the first line, a blank line, then the body.
func FormatPullRequestText(title, body string) string {
	return title + "\n\n" + body
}

// ParsePullRequestText is the reverse of FormatPullRequestText. The first line
// is the title (trimmed); the body is everything after the blank line that
// follows it, with trailing whitespace removed. Line endings may be CRLF, and
// the body comes back with LF. A second line that is not blank is an error
// rather than dropped text.
func ParsePullRequestText(text string) (title, body string, err error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	first, rest, hasRest := strings.Cut(text, "\n")
	title = strings.TrimSpace(first)
	if title == "" {
		return "", "", ErrEmptyTitle
	}
	if !hasRest {
		return title, "", nil
	}
	second, afterBlank, _ := strings.Cut(rest, "\n")
	if strings.TrimSpace(second) != "" {
		return "", "", ErrMissingBlankLine
	}
	return title, strings.TrimRight(afterBlank, " \t\r\n"), nil
}
