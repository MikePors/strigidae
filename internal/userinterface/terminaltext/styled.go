package terminaltext

import (
	"strings"
	"unicode/utf8"
)

// CleanStyled sanitises text that is already styled, such as markdown rendered
// to ANSI, where decoding (for example of HTML character references) may have
// produced new escape sequences after CleanBlock ran. It keeps only SGR
// sequences (ESC [ digits/;/: m) and well-formed OSC 8 hyperlinks whose
// parameters and URL hold no control or bidi runes. Every other escape
// sequence is dropped, and every control, C1 and bidi rune other than "\n"
// becomes a space.
func CleanStyled(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	for i := 0; i < len(text); {
		if text[i] != 0x1b {
			character, count := utf8.DecodeRuneInString(text[i:])
			switch {
			case character == '\n':
				builder.WriteByte('\n')
			case character == '\t': // keeps code indentation readable
				builder.WriteString("    ")
			case character == utf8.RuneError && count == 1: // invalid byte
				builder.WriteRune(utf8.RuneError)
			case unsafeRune(character):
				builder.WriteByte(' ')
			default:
				builder.WriteString(text[i : i+count])
			}
			i += count
			continue
		}
		sequence, count := escapeSequence(text[i:])
		if keepSequence(sequence) {
			builder.WriteString(sequence)
		}
		i += count
	}
	return builder.String()
}

// escapeSequence returns the escape sequence at the start of s (which begins
// with ESC) and its length. A sequence that never ends swallows the rest of text.
func escapeSequence(text string) (string, int) {
	if len(text) < 2 {
		return text, len(text)
	}
	switch text[1] {
	case '[': // CSI: parameter bytes, intermediate bytes, one final byte
		i := 2
		for i < len(text) && text[i] >= 0x30 && text[i] <= 0x3f {
			i++
		}
		for i < len(text) && text[i] >= 0x20 && text[i] <= 0x2f {
			i++
		}
		if i < len(text) && text[i] >= 0x40 && text[i] <= 0x7e {
			i++
		}
		return text[:i], i
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: end at BEL or ST
		for i := 2; i < len(text); i++ {
			switch {
			case text[i] == 0x07:
				return text[:i+1], i + 1
			case text[i] == 0x1b && i+1 < len(text) && text[i+1] == '\\':
				return text[:i+2], i + 2
			}
		}
		return text, len(text)
	default: // two-byte escape, possibly with intermediates
		i := 1
		for i < len(text) && text[i] >= 0x20 && text[i] <= 0x2f {
			i++
		}
		if i < len(text) {
			i++
		}
		return text[:i], i
	}
}

func keepSequence(sequence string) bool {
	switch {
	case strings.HasPrefix(sequence, "\x1b["):
		if !strings.HasSuffix(sequence, "m") {
			return false
		}
		return !strings.ContainsFunc(sequence[2:len(sequence)-1], func(character rune) bool {
			return (character < '0' || character > '9') && character != ';' && character != ':'
		})
	case strings.HasPrefix(sequence, "\x1b]8;"):
		body := sequence[4:]
		switch {
		case strings.HasSuffix(body, "\x07"):
			body = body[:len(body)-1]
		case strings.HasSuffix(body, "\x1b\\"):
			body = body[:len(body)-2]
		default:
			return false
		}
		parameters, url, ok := strings.Cut(body, ";")
		return ok && !strings.Contains(parameters, ";") &&
			!strings.ContainsFunc(parameters+url, func(character rune) bool { return character == 0x1b || unsafeRune(character) })
	}
	return false
}
