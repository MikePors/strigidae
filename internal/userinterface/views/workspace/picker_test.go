package workspace

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestFilePicker(test *testing.T) {
	harness := loaded(test, 100, 24)
	if harness.v.CapturesKeys() {
		test.Fatal("picker should start closed")
	}
	harness.press("f")
	if !harness.v.CapturesKeys() {
		test.Fatal("f should open the picker")
	}

	// Filtering narrows the list; enter jumps to the match.
	harness.typ("retry")
	if got := harness.v.picker.Matches(); got != 2 {
		test.Errorf("matches for %q = %d, want 2 (retry.go, retry-policy doc)", harness.v.picker.Query(), got)
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyDown})
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if harness.v.CapturesKeys() {
		test.Error("enter should close the picker")
	}
	if got := harness.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		test.Errorf("jumped to %q", got)
	}

	// esc cancels and leaves the selection alone.
	harness.press("f")
	harness.typ("read")
	harness.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if harness.v.CapturesKeys() {
		test.Error("esc should close the picker")
	}
	if got := harness.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		test.Errorf("esc changed selection to %q", got)
	}

	// Typed letters are filter text, not commands; backspace edits.
	harness.press("f")
	harness.typ("jl")
	if harness.v.focus != paneFiles {
		test.Error("typing in the picker leaked into the view")
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	harness.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if harness.v.picker.Matches() != 5 {
		test.Errorf("empty filter should list every file, got %d", harness.v.picker.Matches())
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func TestFilePickerNoMatchEnter(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("f")
	harness.typ("zzzz")
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !harness.v.CapturesKeys() {
		test.Error("enter with no matches should keep the picker open")
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

// TestCapturerReleasesOnEsc is the workspace's entry for the esc-releases
// capture rule (see userinterface.TestCapturersReleaseOnEsc, which cannot import views),
// driven through the real root so key routing is covered too.
func TestCapturerReleasesOnEsc(test *testing.T) {
	harness := loaded(test, 100, 24)
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	model := userinterface.New(harness.v, theme.New(true), userinterface.Options{Tick: noTick})
	step := func(message tea.Msg) {
		test.Helper()
		_, _ = model.Update(message) // commands (timers, spinner) are not run
	}
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	step(tea.KeyPressMsg{Code: '}', Text: "}"})
	if harness.v.sel() != 1 {
		test.Fatalf("} through the root: sel = %d", harness.v.sel())
	}
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !harness.v.CapturesKeys() {
		test.Fatal("f through the root should open the picker")
	}
	step(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if harness.v.picker.Query() != "q" {
		test.Errorf("key did not reach the picker raw: %q", harness.v.picker.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if harness.v.CapturesKeys() {
		test.Error("view still captures keys after esc")
	}
}

func TestPickerChoosesByPathAcrossRefresh(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("f")
	harness.typ("legacy")
	// A refresh lands while the picker is open, with a new file sorted first.
	detail := testDetail()
	detail.Files = append([]github.ChangedFile{{Path: "aaa/new.go", Additions: 1, Change: github.FileAdded}}, detail.Files...)
	harness.send(pullRequestLoadedMessage{owner: harness.v, repository: testRepository, number: 7, sequence: harness.v.sequence, detail: detail})
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := harness.v.selectedPath(); got != "internal/sync/legacy.go" {
		test.Errorf("picker selected %q, want legacy.go", got)
	}
}
