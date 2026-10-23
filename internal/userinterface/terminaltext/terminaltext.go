// Package termtext sanitises untrusted text (anything that comes from GitHub
// or git) before it is drawn, so it cannot carry terminal escape sequences or
// reorder the display with bidirectional controls.
package terminaltext

import (
	"strings"
	"unicode"
)

// Clean prepares single-line text such as titles, authors, labels, branch
// names and paths. Every control rune (C0, DEL, C1, including newline and tab)
// and every bidi control rune becomes a space, so the text stays on one line.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
}

// CleanBlock prepares multi-line text such as PR descriptions and comment
// bodies before markdown rendering. It behaves like Clean but keeps newlines
// and tabs, and turns "\r\n" and lone "\r" into "\n".
func CleanBlock(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unsafeRune(r) {
			return ' '
		}
		return r
	}, s)
}

func unsafeRune(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
}
