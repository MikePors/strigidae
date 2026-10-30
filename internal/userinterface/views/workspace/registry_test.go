package workspace

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// A key and its command reach the same entry.
func TestKeyAndCommandReachTheSameEntry(test *testing.T) {
	view := newHarness(test, 100, 24).view
	reached := 0
	for _, candidate := range view.bind.entries() {
		if candidate.Binding == nil || candidate.Command == "" {
			continue
		}
		reached++
		fromCommand, ok := view.commandEntry(candidate.Command)
		if !ok || fromCommand.Name != candidate.Name {
			test.Errorf(":%s reaches %q, want %q", candidate.Command, fromCommand.Name, candidate.Name)
		}
		pressed := candidate.Binding.Keys()[0]
		fromKey, ok := view.keyEntry(keys.Message{Sequence: pressed})
		if !ok || fromKey.Name != candidate.Name {
			test.Errorf("%q reaches %q, want %q", pressed, fromKey.Name, candidate.Name)
		}
	}
	if reached == 0 {
		test.Error("no entry has both a key and a command")
	}
}

// What an entry declares has to hang together.
func TestEntriesAreConsistent(test *testing.T) {
	bindings := defaultBindings()
	for _, candidate := range bindings.entries() {
		test.Run(candidate.Name, func(test *testing.T) {
			if candidate.run == nil && candidate.Binding == nil {
				test.Error("an entry without keys needs run")
			}
			if candidate.run == nil && candidate.Command != "" {
				test.Error("a command needs run")
			}
			if candidate.level.resolved() && !candidate.diffVisible {
				test.Error("a diff-cursor level needs the diff pane on screen")
			}
			if candidate.level.resolved() && candidate.words == "" {
				test.Error("a diff-cursor level needs words for its notices")
			}
			if candidate.fresh != freshNone && candidate.guard == writeNone {
				test.Error("a fresh check without a guard kind: only writes re-read")
			}
		})
	}
}

// Commands lists exactly the commands the view handles; anything else is not
// handled, so the shell can say "not a command".
func TestCommandsAreHandledAndOthersAreNot(test *testing.T) {
	view := newHarness(test, 100, 24).view
	names := Commands()
	for _, name := range names {
		if _, handled := view.Command(name, ""); !handled {
			test.Errorf(":%s is listed but not handled", name)
		}
	}
	if command, handled := view.Command("frobnicate", ""); handled || command != nil {
		test.Error("an unknown command was handled")
	}
	want := []string{"approve", "checks", "copy-filename", "delete-comment", "discard-review", "edit-comment", "merge", "react", "request-changes", "resolve", "review", "unresolve"}
	if diff := cmp.Diff(want, slices.Sorted(slices.Values(names))); diff != "" {
		test.Errorf("commands (-want +got):\n%s", diff)
	}
}

// ] and [ belong to the Overview's sections; in a file they fall through to
// the diff pane's hunks.
func TestEntryThatDoesNotApplyFallsThrough(test *testing.T) {
	harness := loaded(test, 100, 24)
	view := harness.view
	next := keys.Message{Sequence: view.bind.NextSection.Keys()[0]}
	if _, ok := view.keyEntry(next); !ok {
		test.Fatal("the section key should reach next-section on the Overview")
	}
	harness.press("j") // select the first file
	if view.selection() == 0 {
		test.Fatal("a file should be selected")
	}
	if reached, ok := view.keyEntry(next); ok {
		test.Errorf("the section key reached %q in a file", reached.Name)
	}
}
