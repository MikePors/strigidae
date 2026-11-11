package pullrequestlist

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/userinterface"
)

type fakeCopier struct{ texts []string }

func (copier *fakeCopier) Copy(_ context.Context, text string) error {
	copier.texts = append(copier.texts, text)
	return nil
}

func copiedView(test *testing.T, pullRequestCount int) (*View, *fakeCopier) {
	test.Helper()
	view := loadedView(test, fakeWith(makePullRequests(pullRequestCount)), nil)
	copier := &fakeCopier{}
	view.clipboard = copier
	return view, copier
}

// copyURLMessages runs :copy-url and returns what it produced.
func copyURLMessages(test *testing.T, view *View) []tea.Msg {
	test.Helper()
	command, handled := view.Command("copy-url", "")
	if !handled {
		test.Fatal("copy-url not handled")
	}
	var produced []tea.Msg
	for _, message := range flatten(command()) {
		if message != nil {
			produced = append(produced, message)
		}
	}
	return produced
}

func TestCopyURLCopiesTheRowUnderTheCursor(test *testing.T) {
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"first row", nil, "https://github.com/acme/widgets/pull/3"},
		{"after moving down", []string{"j"}, "https://github.com/acme/widgets/pull/2"},
		{"at the bottom", []string{"G"}, "https://github.com/acme/widgets/pull/1"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view, copier := copiedView(test, 3)
			for _, sequence := range testCase.keys {
				press(view, sequence)
			}
			produced := copyURLMessages(test, view)
			if len(produced) != 2 || produced[0] != tea.SetClipboard(testCase.want)() {
				test.Fatalf("messages = %#v, want OSC 52 for %q first", produced, testCase.want)
			}
			if diff := cmp.Diff(userinterface.InfoMessage{Text: "Copied " + testCase.want}, produced[1]); diff != "" {
				test.Errorf("status (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{testCase.want}, copier.texts); diff != "" {
				test.Errorf("piped (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCopyURLWithNoRow(test *testing.T) {
	for name, loaded := range map[string]bool{"loading": false, "empty": true} {
		test.Run(name, func(test *testing.T) {
			view := newView(fakeWith(nil), nil)
			copier := &fakeCopier{}
			view.clipboard = copier
			if loaded {
				deliver(test, view, view.Init())
			}
			produced := copyURLMessages(test, view)
			if len(produced) != 1 {
				test.Fatalf("messages = %#v", produced)
			}
			if failure, ok := produced[0].(userinterface.ErrorMessage); !ok || failure.Err.Error() != "no pull request URL to copy" {
				test.Errorf("message = %#v", produced[0])
			}
			if len(copier.texts) != 0 {
				test.Errorf("piped %q", copier.texts)
			}
		})
	}
}

func TestCopyURLRefusesControlCharacters(test *testing.T) {
	view, copier := copiedView(test, 3)
	view.current().pullRequests[0].URL = "https://x/\x1b]52;c;x\x07y"
	produced := copyURLMessages(test, view)
	if len(produced) != 1 {
		test.Fatalf("messages = %#v", produced)
	}
	failure, ok := produced[0].(userinterface.ErrorMessage)
	if !ok || !strings.Contains(failure.Err.Error(), "control characters") || strings.ContainsAny(failure.Err.Error(), "\x1b\x07") {
		test.Errorf("message = %#v", produced[0])
	}
	if len(copier.texts) != 0 {
		test.Errorf("piped %q", copier.texts)
	}
}

func TestCopyURLUsesTheShownTab(test *testing.T) {
	client := fakeWith(makePullRequests(2))
	client.SetSearchResults("is:pr is:open repo:acme/widgets author:alice sort:updated-desc", authored("alice", 21))
	view := tabbedWith(test, client, config.Tab{Name: "Alice", User: "alice"})
	copier := &fakeCopier{}
	view.clipboard = copier
	deliver(test, view, press(view, "g t"))
	copyURLMessages(test, view)
	if diff := cmp.Diff([]string{"https://example.test/alice"}, copier.texts); diff != "" {
		test.Errorf("piped (-want +got):\n%s", diff)
	}
}
