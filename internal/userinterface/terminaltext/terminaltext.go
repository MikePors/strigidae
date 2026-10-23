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
func Clean(text string) string {
	return strings.Map(func(character rune) rune {
		if unsafeRune(character) {
			return ' '
		}
		return character
	}, text)
}

// CleanBlock prepares multi-line text such as PR descriptions and comment
// bodies before markdown rendering. It behaves like Clean but keeps newlines
// and tabs, and turns "\r\n" and lone "\r" into "\n".
func CleanBlock(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return character
		}
		if unsafeRune(character) {
			return ' '
		}
		return character
	}, text)
}

func unsafeRune(character rune) bool {
	return unicode.IsControl(character) || unicode.Is(unicode.Bidi_Control, character)
}
