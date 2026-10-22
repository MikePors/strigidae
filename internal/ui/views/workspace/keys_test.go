package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// terminalKeys returns the key presses a terminal can produce for a shifted
// punctuation character: plain text (the legacy encoding, which is what
// Bubble Tea v2 gets by default) and the kitty encoding that reports the base
// key plus Shift.
func terminalKeys(s string, base rune) map[string]tea.KeyPressMsg {
	return map[string]tea.KeyPressMsg{
		"text":  {Code: rune(s[0]), Text: s},
		"kitty": {Code: base, ShiftedCode: rune(s[0]), Mod: tea.ModShift, Text: s},
	}
}

// rootFor wraps the harness's view in the real root and counts sequence
// timers, so a test can prove a key never starts a pending prefix.
func rootFor(h *harness) (m *ui.Model, timers *int) {
	timers = new(int)
	tick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { *timers++; return nil }
	m = ui.New(h.v, theme.New(true), ui.Options{Tick: tick})
	_, _ = m.Update(tea.WindowSizeMsg{Width: h.v.width, Height: h.v.height + 1})
	return m, timers
}

func TestNavigationKeysAreReportedAsText(t *testing.T) {
	bases := map[string]rune{"}": ']', "{": '[', ")": '0', "(": '9'}
	for s, base := range bases {
		for name, k := range terminalKeys(s, base) {
			if got := k.String(); got != s {
				t.Errorf("%s %q stringifies as %q", name, s, got)
			}
		}
	}
}

func TestNavigationKeysThroughRoot(t *testing.T) {
	press := func(m *ui.Model, s string) {
		t.Helper()
		_, _ = m.Update(tea.KeyPressMsg{Code: rune(s[0]), Text: s})
	}

	t.Run("} and { move between files from Overview", func(t *testing.T) {
		for name, k := range terminalKeys("}", ']') {
			h := loaded(t, 100, 24)
			m, timers := rootFor(h)
			_, _ = m.Update(k)
			if h.v.sel() != 1 {
				t.Errorf("%s: } sel = %d, want 1", name, h.v.sel())
			}
			_, _ = m.Update(terminalKeys("{", '[')[name])
			if h.v.sel() != 1 {
				t.Errorf("%s: { at the first file should stay, sel = %d", name, h.v.sel())
			}
			if *timers != 0 {
				t.Errorf("%s: %d sequence timers started", name, *timers)
			}
		}
	})

	t.Run("] and [ jump sections in Overview at once", func(t *testing.T) {
		h := loaded(t, 100, 14)
		m, timers := rootFor(h)
		press(m, "l")
		press(m, "]")
		if h.v.ov.vp.YOffset() != h.v.ov.sections[1] {
			t.Errorf("] did not act immediately: offset = %d", h.v.ov.vp.YOffset())
		}
		press(m, "[")
		if h.v.ov.vp.YOffset() != 0 {
			t.Errorf("[: offset = %d", h.v.ov.vp.YOffset())
		}
		if *timers != 0 {
			t.Errorf("] / [ started %d sequence timers", *timers)
		}
	})

	t.Run("] and [ move between hunks in the diff", func(t *testing.T) {
		h := openClientDiff(t, 100, 24)
		m, timers := rootFor(h)
		press(m, "]")
		press(m, "j")
		if !strings.Contains(h.cursorText(), "c.conn.Ping") {
			t.Errorf("]: cursor on %q", h.cursorText())
		}
		press(m, "[") // back to the header of the hunk we are in
		press(m, "[")
		press(m, "j")
		if !strings.Contains(h.cursorText(), `"context"`) {
			t.Errorf("[: cursor on %q", h.cursorText())
		}
		if *timers != 0 {
			t.Errorf("] / [ started %d sequence timers", *timers)
		}
	})

	t.Run(") and ( move between threads in the diff", func(t *testing.T) {
		for name, k := range terminalKeys(")", '0') {
			h := threadHarness(t, 110, 40)
			h.selectFile()
			m, timers := rootFor(h)
			_, _ = m.Update(k)
			if !strings.Contains(h.cursorRow(), "carol") {
				t.Errorf("%s: ) landed on %q", name, h.cursorRow())
			}
			_, _ = m.Update(terminalKeys("(", '9')[name])
			if !strings.Contains(h.cursorRow(), "carol") {
				t.Errorf("%s: ( at the first thread landed on %q", name, h.cursorRow())
			}
			if *timers != 0 {
				t.Errorf("%s: %d sequence timers started", name, *timers)
			}
		}
	})

	t.Run("old sequences do nothing", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("l", "] f", "[ f", "] s", "[ s", "] h", "[ h", "] c", "[ c")
		if h.v.sel() != 0 || h.v.ov.vp.YOffset() != 0 {
			t.Errorf("an old sequence acted: sel = %d, offset = %d", h.v.sel(), h.v.ov.vp.YOffset())
		}
	})
}

// TestNoBracketPrefixSequences proves no binding in the workspace starts a
// bracket sequence, so a bare ] or [ never waits for a second key.
func TestNoBracketPrefixSequences(t *testing.T) {
	h := openClientDiff(t, 120, 30)
	for _, grp := range h.v.FullHelp() {
		for _, b := range grp {
			for _, k := range b.Keys() {
				if strings.HasPrefix(k, "] ") || strings.HasPrefix(k, "[ ") {
					t.Errorf("binding %q is a bracket sequence", k)
				}
			}
		}
	}
}
