package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// The "text" shape is what strig gets today: Bubble Tea v2 pushes only kitty
// flag 1 (disambiguate), so shifted punctuation arrives as plain text. The
// "kitty" shape needs flags 4 (alternate keys) and 16 (associated text) as
// well; with flag 8 (all keys as escape codes) but not 16, Text is empty and
// String() would be "shift+]", which no binding matches. Enabling more flags
// later must keep Text populated or rebind.
//
// terminalKeys returns the key presses a terminal can produce for a shifted
// punctuation character: plain text (the legacy encoding, which is what
// Bubble Tea v2 gets by default) and the kitty encoding that reports the base
// key plus Shift.
func terminalKeys(text string, base rune) map[string]tea.KeyPressMsg {
	return map[string]tea.KeyPressMsg{
		"text":  {Code: rune(text[0]), Text: text},
		"kitty": {Code: base, ShiftedCode: rune(text[0]), Mod: tea.ModShift, Text: text},
	}
}

// rootFor wraps the harness's view in the real root and counts sequence
// timers, so a test can prove a key never starts a pending prefix.
func rootFor(harness *harness) (model *userinterface.Model, timers *int) {
	timers = new(int)
	tick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { *timers++; return nil }
	model = userinterface.New(harness.view, theme.New(true), userinterface.Options{Tick: tick})
	_, _ = model.Update(tea.WindowSizeMsg{Width: harness.view.width, Height: harness.view.height + 1})
	return model, timers
}

func TestNavigationKeysAreReportedAsText(test *testing.T) {
	bases := map[string]rune{"}": ']', "{": '[', ")": '0', "(": '9'}
	for text, base := range bases {
		for name, key := range terminalKeys(text, base) {
			if got := key.String(); got != text {
				test.Errorf("%s %q stringifies as %q", name, text, got)
			}
		}
	}
}

func TestNavigationKeysThroughRoot(test *testing.T) {
	press := func(model *userinterface.Model, text string) {
		test.Helper()
		_, _ = model.Update(tea.KeyPressMsg{Code: rune(text[0]), Text: text})
	}

	test.Run("} and { move between files from Overview", func(test *testing.T) {
		for name, key := range terminalKeys("}", ']') {
			harness := loaded(test, 100, 24)
			model, timers := rootFor(harness)
			_, _ = model.Update(key)
			if harness.view.selection() != 1 {
				test.Errorf("%s: } sel = %d, want 1", name, harness.view.selection())
			}
			_, _ = model.Update(terminalKeys("{", '[')[name])
			if harness.view.selection() != 1 {
				test.Errorf("%s: { at the first file should stay, sel = %d", name, harness.view.selection())
			}
			if *timers != 0 {
				test.Errorf("%s: %d sequence timers started", name, *timers)
			}
		}
	})

	test.Run("] and [ jump sections in Overview at once", func(test *testing.T) {
		harness := loaded(test, 100, 14)
		model, timers := rootFor(harness)
		press(model, "l")
		press(model, "]")
		if harness.view.overview.viewport.YOffset() != harness.view.overview.sections[1] {
			test.Errorf("] did not act immediately: offset = %d", harness.view.overview.viewport.YOffset())
		}
		press(model, "[")
		if harness.view.overview.viewport.YOffset() != 0 {
			test.Errorf("[: offset = %d", harness.view.overview.viewport.YOffset())
		}
		if *timers != 0 {
			test.Errorf("] / [ started %d sequence timers", *timers)
		}
	})

	test.Run("] and [ move between hunks in the diff", func(test *testing.T) {
		harness := openClientDiff(test, 100, 24)
		model, timers := rootFor(harness)
		press(model, "]")
		press(model, "j")
		if !strings.Contains(harness.cursorText(), "c.conn.Ping") {
			test.Errorf("]: cursor on %q", harness.cursorText())
		}
		press(model, "[") // back to the header of the hunk we are in
		press(model, "[")
		press(model, "j")
		if !strings.Contains(harness.cursorText(), `"context"`) {
			test.Errorf("[: cursor on %q", harness.cursorText())
		}
		if *timers != 0 {
			test.Errorf("] / [ started %d sequence timers", *timers)
		}
	})

	test.Run(") and ( move between threads in the diff", func(test *testing.T) {
		for name, key := range terminalKeys(")", '0') {
			harness := threadHarness(test, 110, 40)
			harness.selectFile()
			model, timers := rootFor(harness)
			_, _ = model.Update(key)
			if !strings.Contains(harness.cursorRow(), "carol") {
				test.Errorf("%s: ) landed on %q", name, harness.cursorRow())
			}
			_, _ = model.Update(terminalKeys("(", '9')[name])
			if !strings.Contains(harness.cursorRow(), "carol") {
				test.Errorf("%s: ( at the first thread landed on %q", name, harness.cursorRow())
			}
			if *timers != 0 {
				test.Errorf("%s: %d sequence timers started", name, *timers)
			}
		}
	})

	test.Run("old sequences are just two keys", func(test *testing.T) {
		harness := loaded(test, 100, 14)
		model, _ := rootFor(harness)
		press(model, "l")
		press(model, "]") // jumps a section at once
		if harness.view.overview.viewport.YOffset() != harness.view.overview.sections[1] {
			test.Errorf("]: offset = %d", harness.view.overview.viewport.YOffset())
		}
		press(model, "f") // is the file picker, not "next file"
		if !harness.view.CapturesKeys() || harness.view.selection() != 0 {
			test.Errorf("f after ]: captures = %v, sel = %d", harness.view.CapturesKeys(), harness.view.selection())
		}
	})

	test.Run("hunk keys work from the Files pane", func(test *testing.T) {
		harness := openClientDiff(test, 100, 24)
		model, _ := rootFor(harness)
		press(model, "h")
		press(model, "]")
		press(model, "l")
		press(model, "j")
		if !strings.Contains(harness.cursorText(), "c.conn.Ping") {
			test.Errorf("] from the Files pane: cursor on %q", harness.cursorText())
		}
	})
}

// TestHelpKeysMatchBindings keeps each help key equal to a real key.
func TestHelpKeysMatchBindings(test *testing.T) {
	harness := openClientDiff(test, 100, 24)
	for _, bindings := range harness.view.FullHelp() {
		for _, binding := range bindings {
			if binding.Help().Key == "" {
				continue
			}
			ok := false
			for _, key := range binding.Keys() {
				ok = ok || strings.ReplaceAll(key, " ", "") == binding.Help().Key || key == binding.Help().Key
			}
			if !ok {
				test.Errorf("help key %q is not one of %v", binding.Help().Key, binding.Keys())
			}
		}
	}
}

// TestNoBracketPrefixSequences proves no binding in the workspace starts a
// bracket sequence, so a bare ] or [ never waits for a second key.
func TestNoBracketPrefixSequences(test *testing.T) {
	harness := openClientDiff(test, 120, 30)
	for _, bindings := range harness.view.FullHelp() {
		for _, binding := range bindings {
			for _, key := range binding.Keys() {
				if strings.HasPrefix(key, "] ") || strings.HasPrefix(key, "[ ") {
					test.Errorf("binding %q is a bracket sequence", key)
				}
			}
		}
	}
}
