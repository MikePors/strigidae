package termtext

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
func CleanStyled(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			r, n := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == '\n':
				b.WriteByte('\n')
			case r == '\t': // keeps code indentation readable
				b.WriteString("    ")
			case r == utf8.RuneError && n == 1: // invalid byte
				b.WriteRune(utf8.RuneError)
			case unsafeRune(r):
				b.WriteByte(' ')
			default:
				b.WriteString(s[i : i+n])
			}
			i += n
			continue
		}
		seq, n := escapeSequence(s[i:])
		if keepSequence(seq) {
			b.WriteString(seq)
		}
		i += n
	}
	return b.String()
}

// escapeSequence returns the escape sequence at the start of s (which begins
// with ESC) and its length. A sequence that never ends swallows the rest of s.
func escapeSequence(s string) (string, int) {
	if len(s) < 2 {
		return s, len(s)
	}
	switch s[1] {
	case '[': // CSI: parameter bytes, intermediate bytes, one final byte
		i := 2
		for i < len(s) && s[i] >= 0x30 && s[i] <= 0x3f {
			i++
		}
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
			i++
		}
		return s[:i], i
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: end at BEL or ST
		for i := 2; i < len(s); i++ {
			switch {
			case s[i] == 0x07:
				return s[:i+1], i + 1
			case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\':
				return s[:i+2], i + 2
			}
		}
		return s, len(s)
	default: // two-byte escape, possibly with intermediates
		i := 1
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) {
			i++
		}
		return s[:i], i
	}
}

func keepSequence(seq string) bool {
	switch {
	case strings.HasPrefix(seq, "\x1b["):
		if !strings.HasSuffix(seq, "m") {
			return false
		}
		return !strings.ContainsFunc(seq[2:len(seq)-1], func(r rune) bool {
			return (r < '0' || r > '9') && r != ';' && r != ':'
		})
	case strings.HasPrefix(seq, "\x1b]8;"):
		body := seq[4:]
		switch {
		case strings.HasSuffix(body, "\x07"):
			body = body[:len(body)-1]
		case strings.HasSuffix(body, "\x1b\\"):
			body = body[:len(body)-2]
		default:
			return false
		}
		params, url, ok := strings.Cut(body, ";")
		return ok && !strings.Contains(params, ";") &&
			!strings.ContainsFunc(params+url, func(r rune) bool { return r == 0x1b || unsafeRune(r) })
	}
	return false
}
