package dashboard

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

type fakeCopier struct{ texts []string }

func (copier *fakeCopier) Copy(_ context.Context, text string) error {
	copier.texts = append(copier.texts, text)
	return nil
}

func copiedView(test *testing.T) (*View, *fakeCopier) {
	test.Helper()
	view, _ := loadedView(test)
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
	for _, message := range messages(command) {
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
		{"first row", nil, "https://github.com/acme/widgets/pull/12"},
		{"after moving down", []string{"j"}, "https://github.com/acme/gadgets/pull/7"},
		{"in the second section", []string{"]"}, "https://github.com/rivka/dotfiles/pull/3"},
		{"after moving past the section edge", []string{"j", "j"}, "https://github.com/rivka/dotfiles/pull/3"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view, copier := copiedView(test)
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
	tabs := testTabs(test)
	client := fake.New()
	for _, tab := range tabs {
		for _, section := range tab.Sections {
			client.SetSearchResults(searchQuery(section), nil)
		}
	}
	for name, loaded := range map[string]bool{"loading": false, "empty": true} {
		test.Run(name, func(test *testing.T) {
			view := newView(client, tabs, nil)
			copier := &fakeCopier{}
			view.clipboard = copier
			if loaded {
				settle(view, view.Init())
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
	view, copier := copiedView(test)
	view.tabs[0].sections[0].pullRequests[0].URL = "https://x/\x1b]52;c;x\x07y"
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

func TestCommandNamesAreTheCommandsTheViewHandles(test *testing.T) {
	view, _ := loadedView(test)
	if diff := cmp.Diff(Commands(), view.CommandNames()); diff != "" {
		test.Errorf("command names (-want +got):\n%s", diff)
	}
	for _, name := range view.CommandNames() {
		if _, handled := view.Command(name, ""); !handled {
			test.Errorf("%s listed but not handled", name)
		}
	}
	if _, handled := view.Command("create", ""); handled {
		test.Error("the dashboard handles :create")
	}
}

func TestCopyURLUsesTheShownTab(test *testing.T) {
	view, copier := copiedView(test)
	settle(view, press(view, "g t"))
	copyURLMessages(test, view)
	if diff := cmp.Diff([]string{"https://github.com/acme/widgets/pull/40"}, copier.texts); diff != "" {
		test.Errorf("piped (-want +got):\n%s", diff)
	}
}
