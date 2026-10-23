package workspace

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface"
)

func copyCommand(test *testing.T, view *View) []tea.Msg {
	test.Helper()
	command, ok := view.Command("copy-filename", "")
	if !ok {
		test.Fatal("copy-filename not handled")
	}
	return messages(command)
}

func TestCopyFilename(test *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		focus pane
		want  string
	}{
		{"files pane", []string{"j"}, paneFiles, "internal/sync/client.go"},
		{"files pane, second file", []string{"j", "j"}, paneFiles, "internal/sync/retry.go"},
		{"diff pane", []string{"j", "j", "l"}, paneRight, "internal/sync/retry.go"},
		{"renamed file copies the new path", []string{"j", "j", "j", "j", "l"}, paneRight, "docs/design/a-very-long-directory-name/retry-policy-and-backoff.md"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := loaded(test, 120, 30)
			harness.press(testCase.keys...)
			if harness.v.focus != testCase.focus {
				test.Fatalf("focus = %v, want %v", harness.v.focus, testCase.focus)
			}
			messages := copyCommand(test, harness.v)
			if len(messages) != 2 || messages[0] != tea.SetClipboard(testCase.want)() {
				test.Fatalf("msgs = %#v, want clipboard %q first", messages, testCase.want)
			}
			if info, ok := messages[1].(userinterface.InfoMessage); !ok || info.Text != "Copied "+testCase.want {
				test.Errorf("status = %#v", messages[1])
			}
		})
	}
}

func TestCopyFilenameOverview(test *testing.T) {
	harness := loaded(test, 120, 30)
	messages := copyCommand(test, harness.v)
	if len(messages) != 1 {
		test.Fatalf("msgs = %#v", messages)
	}
	if errorMessage, ok := messages[0].(userinterface.ErrorMessage); !ok || errorMessage.Err.Error() != "no file to copy" {
		test.Errorf("msg = %#v", messages[0])
	}
}

func TestCopyFilenameRefusesControlCharacters(test *testing.T) {
	harness := loaded(test, 120, 30)
	harness.press("j")
	harness.v.detail.Files[0].Path = "a\x1b]52;c;x\x07b.go"
	messages := copyCommand(test, harness.v)
	if len(messages) != 1 {
		test.Fatalf("msgs = %#v", messages)
	}
	errorMessage, ok := messages[0].(userinterface.ErrorMessage)
	if !ok || !strings.Contains(errorMessage.Err.Error(), "control characters") || strings.ContainsAny(errorMessage.Err.Error(), "\x1b\x07") {
		test.Errorf("msg = %#v", messages[0])
	}
}
