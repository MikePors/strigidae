package main

import (
	"slices"
	"testing"
)

// The shell is told the commands of every view, so ":merge" on the dashboard
// says "not available here" instead of "not a command".
func TestViewCommandsAreCollectedFromTheViews(test *testing.T) {
	got := viewCommands()
	for _, name := range []string{"merge", "approve", "copy-filename", "edit-comment", "resolve"} {
		if !slices.Contains(got, name) {
			test.Errorf("viewCommands() lacks %q", name)
		}
	}
	// The shell's own commands win, so a view command with the same name
	// could never be reached.
	for _, name := range []string{"q", "quit", "qa", "qall", "quitall", "set", "theme"} {
		if slices.Contains(got, name) {
			test.Errorf("view command %q is also a shell command", name)
		}
	}
	seen := map[string]bool{}
	for _, name := range got {
		if seen[name] {
			test.Errorf("view command %q is declared twice", name)
		}
		seen[name] = true
	}
}
