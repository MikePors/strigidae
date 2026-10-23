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
	if harness.view.CapturesKeys() {
		test.Fatal("picker should start closed")
	}
	harness.press("f")
	if !harness.view.CapturesKeys() {
		test.Fatal("f should open the picker")
	}

	// Filtering narrows the list; enter jumps to the match.
	harness.typeText("retry")
	if got := harness.view.picker.Matches(); got != 2 {
		test.Errorf("matches for %q = %d, want 2 (retry.go, retry-policy doc)", harness.view.picker.Query(), got)
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyDown})
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if harness.view.CapturesKeys() {
		test.Error("enter should close the picker")
	}
	if got := harness.view.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		test.Errorf("jumped to %q", got)
	}

	// esc cancels and leaves the selection alone.
	harness.press("f")
	harness.typeText("read")
	harness.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if harness.view.CapturesKeys() {
		test.Error("esc should close the picker")
	}
	if got := harness.view.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		test.Errorf("esc changed selection to %q", got)
	}

	// Typed letters are filter text, not commands; backspace edits.
	harness.press("f")
	harness.typeText("jl")
	if harness.view.focus != paneFiles {
		test.Error("typing in the picker leaked into the view")
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	harness.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if harness.view.picker.Matches() != 5 {
		test.Errorf("empty filter should list every file, got %d", harness.view.picker.Matches())
	}
	harness.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func TestFilePickerNoMatchEnter(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("f")
	harness.typeText("zzzz")
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !harness.view.CapturesKeys() {
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
	model := userinterface.New(harness.view, theme.New(true), userinterface.Options{Tick: noTick})
	step := func(message tea.Msg) {
		test.Helper()
		_, _ = model.Update(message) // commands (timers, spinner) are not run
	}
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	step(tea.KeyPressMsg{Code: '}', Text: "}"})
	if harness.view.selection() != 1 {
		test.Fatalf("} through the root: sel = %d", harness.view.selection())
	}
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !harness.view.CapturesKeys() {
		test.Fatal("f through the root should open the picker")
	}
	step(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if harness.view.picker.Query() != "q" {
		test.Errorf("key did not reach the picker raw: %q", harness.view.picker.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if harness.view.CapturesKeys() {
		test.Error("view still captures keys after esc")
	}
}

func TestPickerChoosesByPathAcrossRefresh(test *testing.T) {
	harness := loaded(test, 100, 24)
	harness.press("f")
	harness.typeText("legacy")
	// A refresh lands while the picker is open, with a new file sorted first.
	detail := testDetail()
	detail.Files = append([]github.ChangedFile{{Path: "aaa/new.go", Additions: 1, Change: github.FileAdded}}, detail.Files...)
	harness.send(pullRequestLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: harness.view.sequence, detail: detail})
	harness.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := harness.view.selectedPath(); got != "internal/sync/legacy.go" {
		test.Errorf("picker selected %q, want legacy.go", got)
	}
}
