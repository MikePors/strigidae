package terminaltext

import "testing"

func TestClean(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "plain title", "plain title"},
		{"newline and tab", "tab\there\nnewline", "tab here newline"},
		{"carriage return", "a\r\nb\rc", "a  b c"},
		{"C0", "bell\x07nul\x00", "bell nul "},
		{"DEL", "del\x7fchar", "del char"},
		{"C1 csi", "c1\u009bcsi", "c1 csi"},
		{"CSI clear screen", "x\x1b[2Jy", "x [2Jy"},
		{"OSC 52 clipboard", "x\x1b]52;c;aGk=\x07y", "x ]52;c;aGk= y"},
		{"bidi override", "rtl\u202eevil", "rtl evil"},
		{"bidi isolates", "iso\u2066late\u2069d", "iso late d"},
		{"unicode kept", "unicode ✓ ok", "unicode ✓ ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanBlock(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"keeps newline and tab", "a\n\tb", "a\n\tb"},
		{"crlf", "a\r\nb", "a\nb"},
		{"lone cr", "a\rb", "a\nb"},
		{"mixed endings", "1\r\n2\r3\n4", "1\n2\n3\n4"},
		{"C0", "bell\x07nul\x00", "bell nul "},
		{"DEL", "del\x7fchar", "del char"},
		{"C1 csi", "c1\u009bcsi", "c1 csi"},
		{"CSI clear screen", "x\x1b[2Jy", "x [2Jy"},
		{"OSC 52 clipboard", "x\x1b]52;c;aGk=\x07y", "x ]52;c;aGk= y"},
		{"bidi override", "rtl\u202eevil\n", "rtl evil\n"},
		{"bidi isolates", "iso\u2066late\u2069d", "iso late d"},
		{"markdown kept", "# Title\n\n- `code` **bold**", "# Title\n\n- `code` **bold**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanBlock(tt.in); got != tt.want {
				t.Errorf("CleanBlock(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
