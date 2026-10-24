package terminalpalette

import (
	"errors"
	"image/color"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func readFixture(test *testing.T, name string) []byte {
	test.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		test.Fatal(err)
	}
	return data
}

// hexOf formats a colour as "#rrggbb", or "" for nil.
func hexOf(colour color.Color) string {
	if colour == nil {
		return ""
	}
	return theme.HexString(colour)
}

// summary is a Reply as plain data, for cmp.
type summary struct {
	Foreground, Background string
	ANSI                   []string
	Finished               bool
}

func summarise(reply Reply) summary {
	result := summary{Foreground: hexOf(reply.Foreground), Background: hexOf(reply.Background), Finished: reply.Finished}
	for _, colour := range reply.ANSI {
		result.ANSI = append(result.ANSI, hexOf(colour))
	}
	return result
}

var tokyoNight = summary{
	Foreground: "#c0caf5",
	Background: "#1a1b26",
	ANSI: []string{
		"#15161e", "#f7768e", "#9ece6a", "#e0af68", "#7aa2f7", "#bb9af7", "#7dcfff", "#a9b1d6",
		"#414868", "#ff899d", "#b9f27c", "#ff9e64", "#7da6ff", "#c0a3ff", "#b4f9f8", "#c0caf5",
	},
	Finished: true,
}

func TestParseRecorded(test *testing.T) {
	empty := make([]string, 16)
	partial := summary{Foreground: "#c0caf5", Background: "#1a1b26", ANSI: empty, Finished: true}
	tests := []struct {
		fixture string
		want    summary
		wantErr string // from Theme, "" for success
	}{
		{"full-st.reply", tokyoNight, ""},
		{"full-bel.reply", tokyoNight, ""},
		{"partial.reply", partial, "reported only 2 of 18 colours"},
		{"da1-only.reply", summary{ANSI: empty, Finished: true}, "does not report its colours"},
		{"none.reply", summary{ANSI: empty}, "no answer from the terminal"},
	}
	for _, testCase := range tests {
		test.Run(testCase.fixture, func(test *testing.T) {
			reply := Parse(readFixture(test, testCase.fixture))
			if diff := cmp.Diff(testCase.want, summarise(reply)); diff != "" {
				test.Errorf("reply mismatch (-want +got):\n%s", diff)
			}
			built, err := reply.Theme()
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					test.Errorf("Theme() err = %v, want one containing %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if !built.Chosen || !built.FromTerminal || !built.IsDark {
				test.Errorf("Chosen %v, FromTerminal %v, IsDark %v; want all true", built.Chosen, built.FromTerminal, built.IsDark)
			}
			if got := hexOf(built.Roles.Background); got != tokyoNight.Background {
				test.Errorf("background role = %s, want the terminal's %s", got, tokyoNight.Background)
			}
		})
	}
}

func TestParseSkipsNoise(test *testing.T) {
	// Typed keys, an unrelated CSI reply, an unknown OSC and an incomplete
	// trailing answer around one real answer.
	data := "jk\x1b[1;1R\x1b]52;c;aGk=\a\x1b]11;rgb:ff/ff/ff\x1b\\x\x1b]10;rgb:00/00"
	reply := Parse([]byte(data))
	if got := hexOf(reply.Background); got != "#ffffff" {
		test.Errorf("background = %q, want #ffffff", got)
	}
	if reply.Foreground != nil || reply.Finished {
		test.Errorf("foreground %v, finished %v; want nil and false", reply.Foreground, reply.Finished)
	}
}

func TestParseStrayIntroducer(test *testing.T) {
	// alt+] typed during the wait sends "ESC ]" just before the answers.
	reply := Parse(append([]byte("\x1b]"), readFixture(test, "full-st.reply")...))
	if diff := cmp.Diff(tokyoNight, summarise(reply)); diff != "" {
		test.Errorf("reply mismatch (-want +got):\n%s", diff)
	}
}

func TestParseColour(test *testing.T) {
	tests := []struct {
		specification string
		want          string
	}{
		{"rgb:ff/00/80", "#ff0080"},
		{"rgb:ffff/0000/8080", "#ff0080"},
		{"rgb:f/0/8", "#ff0088"},
		{"rgb:fff/000/800", "#ff0080"},
		{"rgba:ffff/0000/8080/ffff", "#ff0080"},
		{"rgb:ff/00", ""},
		{"rgb:ff/00/zz", ""},
		{"rgb:fffff/0/0", ""},
		{"rgb://0", ""},
		{"#ff0080", ""},
		{"", ""},
	}
	for _, testCase := range tests {
		if got := hexOf(parseColour(testCase.specification)); got != testCase.want {
			test.Errorf("parseColour(%q) = %q, want %q", testCase.specification, got, testCase.want)
		}
	}
}

func TestQueries(test *testing.T) {
	plain := string(Queries(false))
	if !strings.HasPrefix(plain, "\x1b]10;?\x1b\\\x1b]11;?\x1b\\\x1b]4;0;?\x1b\\") || !strings.HasSuffix(plain, "\x1b]4;15;?\x1b\\\x1b[c") {
		test.Errorf("queries = %q", plain)
	}
	if count := strings.Count(plain, "\x1b]4;"); count != 16 {
		test.Errorf("%d OSC 4 queries, want 16", count)
	}

	wrapped := string(Queries(true))
	if count := strings.Count(wrapped, "\x1bPtmux;"); count != 19 {
		test.Errorf("%d passthrough wrappers, want 19 (18 colours and DA1)", count)
	}
	if !strings.HasPrefix(wrapped, "\x1bPtmux;\x1b\x1b]10;?\x1b\x1b\\\x1b\\") || !strings.HasSuffix(wrapped, "\x1bPtmux;\x1b\x1b[c\x1b\\") {
		test.Errorf("wrapped queries = %q", wrapped)
	}
}

// fakeTerminal plays back answers, one chunk per Read. Each Read advances the
// clock by step, as a real read waits; an empty chunk is a read that timed out.
type fakeTerminal struct {
	chunks  []string
	clock   *fakeClock
	step    time.Duration
	written strings.Builder
	reads   int
	waits   []time.Duration
	readErr error
}

func (terminal *fakeTerminal) Write(data []byte) (int, error) {
	return terminal.written.Write(data)
}

func (terminal *fakeTerminal) Read(buffer []byte, wait time.Duration) (int, error) {
	terminal.reads++
	terminal.waits = append(terminal.waits, wait)
	terminal.clock.now = terminal.clock.now.Add(min(terminal.step, wait))
	if terminal.readErr != nil {
		return 0, terminal.readErr
	}
	if len(terminal.chunks) == 0 {
		return 0, nil
	}
	chunk := terminal.chunks[0]
	terminal.chunks = terminal.chunks[1:]
	return copy(buffer, chunk), nil
}

type fakeClock struct{ now time.Time }

func (clock *fakeClock) Now() time.Time { return clock.now }

func TestAsk(test *testing.T) {
	full := string(readFixture(test, "full-st.reply"))
	tests := []struct {
		name         string
		chunks       []string
		insideTmux   bool
		step         time.Duration // how long each read takes; default 100ms
		want         summary
		wantMaxReads int
		wantErr      string // from Theme
	}{
		{
			name:         "full answer split mid-sequence stops at DA1",
			chunks:       []string{full[:7], full[7:200], "", full[200:], "never read"},
			step:         10 * time.Millisecond,
			want:         tokyoNight,
			wantMaxReads: 4,
		},
		{
			name:         "DA1 only stops at once",
			chunks:       []string{"\x1b[?1;2c"},
			want:         summary{ANSI: make([]string, 16), Finished: true},
			wantMaxReads: 1,
			wantErr:      "does not report its colours",
		},
		{
			name:         "no answer waits out the timeout",
			want:         summary{ANSI: make([]string, 16)},
			wantMaxReads: 2, // 100ms per read, 200ms timeout
			wantErr:      "no answer from the terminal",
		},
		{
			name:         "the last read is cut short to fit the timeout",
			step:         ReadWait,
			chunks:       []string{"", "x", ""}, // 100ms + 30ms of reads, then 70ms left
			want:         summary{ANSI: make([]string, 16)},
			wantMaxReads: 3,
			wantErr:      "no answer",
		},
		{
			name:         "no answer inside tmux suggests passthrough",
			insideTmux:   true,
			want:         summary{ANSI: make([]string, 16)},
			wantMaxReads: 2,
			wantErr:      "allow-passthrough on",
		},
		{
			name:         "a trickle of noise does not extend the timeout",
			chunks:       []string{"a", "b", "c", "d", "e"},
			want:         summary{ANSI: make([]string, 16)},
			wantMaxReads: 2,
			wantErr:      "no answer",
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			clock := &fakeClock{now: time.Unix(1000, 0)}
			step := testCase.step
			if step == 0 {
				step = 100 * time.Millisecond
			}
			terminal := &fakeTerminal{chunks: testCase.chunks, clock: clock, step: step}
			reply, err := Ask(terminal, testCase.insideTmux, clock.Now)
			if err != nil {
				test.Fatal(err)
			}
			if terminal.written.String() != string(Queries(testCase.insideTmux)) {
				test.Errorf("wrote %q, want the queries", terminal.written.String())
			}
			if diff := cmp.Diff(testCase.want, summarise(reply)); diff != "" {
				test.Errorf("reply mismatch (-want +got):\n%s", diff)
			}
			for _, wait := range terminal.waits {
				if wait <= 0 || wait > ReadWait {
					test.Errorf("a read waited %v, want 0 < wait <= %v", wait, ReadWait)
				}
			}
			if elapsed := clock.now.Sub(time.Unix(1000, 0)); elapsed > Timeout {
				test.Errorf("asking took %v, want at most %v", elapsed, Timeout)
			}
			if terminal.reads > testCase.wantMaxReads {
				test.Errorf("%d reads, want at most %d", terminal.reads, testCase.wantMaxReads)
			}
			_, err = reply.Theme()
			if testCase.wantErr == "" && err != nil {
				test.Errorf("Theme() err = %v", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				test.Errorf("Theme() err = %v, want one containing %q", err, testCase.wantErr)
			}
		})
	}
}

func TestAskReadError(test *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	terminal := &fakeTerminal{clock: clock, readErr: errors.New("gone")}
	if _, err := Ask(terminal, false, clock.Now); err == nil || !strings.Contains(err.Error(), "gone") {
		test.Errorf("err = %v, want the read error", err)
	}
}
