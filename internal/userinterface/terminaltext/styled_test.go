package terminaltext

import "testing"

func TestCleanStyled(test *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "plain text\nline two", "plain text\nline two"},
		{"sgr kept", "\x1b[1;38;5;12mbold\x1b[0m", "\x1b[1;38;5;12mbold\x1b[0m"},
		{"sgr colon form kept", "\x1b[38:2:1:2:3mx\x1b[m", "\x1b[38:2:1:2:3mx\x1b[m"},
		{"clear screen dropped", "a\x1b[2Jb", "ab"},
		{"cursor move dropped", "a\x1b[10;5Hb", "ab"},
		{"private mode dropped", "a\x1b[?1049hb", "ab"},
		{"sgr with intermediate dropped", "a\x1b[1 mb", "ab"},
		{"csi with other final dropped", "a\x1b[1xm\x1b[3mb", "am\x1b[3mb"},
		{"osc 52 dropped", "a\x1b]52;c;aGk=\x07b", "ab"},
		{"osc 52 with st dropped", "a\x1b]52;c;aGk=\x1b\\b", "ab"},
		{"osc title dropped", "a\x1b]0;pwned\x07b", "ab"},
		{"dcs dropped", "a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"apc dropped", "a\x1b_payload\x1b\\b", "ab"},
		{"two-byte escape dropped", "a\x1bcb\x1b7c", "abc"},
		{"unterminated osc swallows the rest", "a\x1b]52;c;xx", "a"},
		{"lone esc at end", "a\x1b", "a"},
		{"osc 8 bel kept", "\x1b]8;id=1;http://x/y\x07link\x1b]8;;\x07", "\x1b]8;id=1;http://x/y\x07link\x1b]8;;\x07"},
		{"osc 8 st kept", "\x1b]8;;http://x\x1b\\l\x1b]8;;\x1b\\", "\x1b]8;;http://x\x1b\\l\x1b]8;;\x1b\\"},
		{"osc 8 with bidi url dropped", "\x1b]8;;http://x/\u202e\x07l", "l"},
		{"osc 8 with c1 url dropped", "\x1b]8;;http://x/\u009b\x07l", "l"},
		{"osc 8 url may hold semicolons", "\x1b]8;a;b;c\x07l", "\x1b]8;a;b;c\x07l"},
		{"osc 8 unterminated dropped", "\x1b]8;;http://x", ""},
		{"c0 becomes space", "bell\x07nul\x00del\x7f", "bell nul del "},
		{"tab becomes four spaces, cr one", "a\tb\rc", "a    b c"},
		{"c1 becomes space", "c1\u009bcsi\u0090dcs", "c1 csi dcs"},
		{"bidi becomes space", "rtl\u202eevil\u2066x\u2069\u200f", "rtl evil x  "},
		{"unicode kept", "unicode ✓ ok", "unicode ✓ ok"},
		{"invalid utf8 does not break out", "a\xffb\xfe\x1b[2Jc", "a�b�c"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if got := CleanStyled(testCase.in); got != testCase.want {
				test.Errorf("CleanStyled(%q) = %q, want %q", testCase.in, got, testCase.want)
			}
		})
	}
}
