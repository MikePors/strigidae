package editor

import (
	"errors"
	"strings"
)

// ErrEmptyTitle is returned by ParsePullRequestText when the first line is empty.
var ErrEmptyTitle = errors.New("empty title")

// FormatPullRequestText is the text a pull request is edited as: the title on
// the first line, a blank line, then the body.
func FormatPullRequestText(title, body string) string {
	return title + "\n\n" + body
}

// ParsePullRequestText is the reverse of FormatPullRequestText. The first line
// is the title (trimmed); the body is everything after it, minus one blank line
// right after the title if there is one, with trailing whitespace removed.
// Line endings may be CRLF; the body comes back with LF.
func ParsePullRequestText(text string) (title, body string, err error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	first, rest, _ := strings.Cut(text, "\n")
	title = strings.TrimSpace(first)
	if title == "" {
		return "", "", ErrEmptyTitle
	}
	second, afterSecond, hasMore := strings.Cut(rest, "\n")
	if strings.TrimSpace(second) == "" {
		rest = afterSecond
		if !hasMore {
			rest = ""
		}
	}
	return title, strings.TrimRight(rest, " \t\r\n"), nil
}
