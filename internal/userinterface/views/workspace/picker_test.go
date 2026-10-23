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

func TestFilePicker(t *testing.T) {
	h := loaded(t, 100, 24)
	if h.v.CapturesKeys() {
		t.Fatal("picker should start closed")
	}
	h.press("f")
	if !h.v.CapturesKeys() {
		t.Fatal("f should open the picker")
	}

	// Filtering narrows the list; enter jumps to the match.
	h.typ("retry")
	if got := h.v.picker.Matches(); got != 2 {
		t.Errorf("matches for %q = %d, want 2 (retry.go, retry-policy doc)", h.v.picker.Query(), got)
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyDown})
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.v.CapturesKeys() {
		t.Error("enter should close the picker")
	}
	if got := h.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		t.Errorf("jumped to %q", got)
	}

	// esc cancels and leaves the selection alone.
	h.press("f")
	h.typ("read")
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.v.CapturesKeys() {
		t.Error("esc should close the picker")
	}
	if got := h.v.selectedPath(); !strings.HasSuffix(got, "retry-policy-and-backoff.md") {
		t.Errorf("esc changed selection to %q", got)
	}

	// Typed letters are filter text, not commands; backspace edits.
	h.press("f")
	h.typ("jl")
	if h.v.focus != paneFiles {
		t.Error("typing in the picker leaked into the view")
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	h.send(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if h.v.picker.Matches() != 5 {
		t.Errorf("empty filter should list every file, got %d", h.v.picker.Matches())
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func TestFilePickerNoMatchEnter(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("f")
	h.typ("zzzz")
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !h.v.CapturesKeys() {
		t.Error("enter with no matches should keep the picker open")
	}
	h.send(tea.KeyPressMsg{Code: tea.KeyEscape})
}

// TestCapturerReleasesOnEsc is the workspace's entry for the esc-releases
// capture rule (see userinterface.TestCapturersReleaseOnEsc, which cannot import views),
// driven through the real root so key routing is covered too.
func TestCapturerReleasesOnEsc(t *testing.T) {
	h := loaded(t, 100, 24)
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	m := userinterface.New(h.v, theme.New(true), userinterface.Options{Tick: noTick})
	step := func(msg tea.Msg) {
		t.Helper()
		_, _ = m.Update(msg) // commands (timers, spinner) are not run
	}
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	step(tea.KeyPressMsg{Code: '}', Text: "}"})
	if h.v.sel() != 1 {
		t.Fatalf("} through the root: sel = %d", h.v.sel())
	}
	step(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !h.v.CapturesKeys() {
		t.Fatal("f through the root should open the picker")
	}
	step(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if h.v.picker.Query() != "q" {
		t.Errorf("key did not reach the picker raw: %q", h.v.picker.Query())
	}
	step(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.v.CapturesKeys() {
		t.Error("view still captures keys after esc")
	}
}

func TestPickerChoosesByPathAcrossRefresh(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("f")
	h.typ("legacy")
	// A refresh lands while the picker is open, with a new file sorted first.
	d := testDetail()
	d.Files = append([]github.ChangedFile{{Path: "aaa/new.go", Additions: 1, Change: github.FileAdded}}, d.Files...)
	h.send(pullRequestLoadedMessage{owner: h.v, repository: testRepository, number: 7, sequence: h.v.sequence, detail: d})
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := h.v.selectedPath(); got != "internal/sync/legacy.go" {
		t.Errorf("picker selected %q, want legacy.go", got)
	}
}
