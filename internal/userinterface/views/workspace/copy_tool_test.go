package workspace

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/clipboard"
	"github.com/MikePors/strigidae/internal/userinterface"
)

type fakeCopier struct {
	texts []string
	err   error
}

func (copier *fakeCopier) Copy(_ context.Context, text string) error {
	copier.texts = append(copier.texts, text)
	return copier.err
}

func copyWith(test *testing.T, copier clipboard.Copier) []tea.Msg {
	test.Helper()
	harness := loaded(test, 120, 30)
	WithClipboard(copier)(harness.view)
	harness.press("j")
	return copyCommand(test, harness.view)
}

func TestCopyFilenameUsesClipboardTool(test *testing.T) {
	copier := &fakeCopier{}
	messages := copyWith(test, copier)
	if diff := cmp.Diff([]string{"internal/sync/client.go"}, copier.texts); diff != "" {
		test.Errorf("piped (-want +got):\n%s", diff)
	}
	if len(messages) != 2 || messages[0] != tea.SetClipboard("internal/sync/client.go")() {
		test.Errorf("msgs = %#v, want OSC 52 and the status", messages)
	}
}

func TestCopyFilenameWithoutClipboardTool(test *testing.T) {
	copier := &fakeCopier{err: clipboard.ErrNoTool}
	logged := captureLog(test)
	messages := copyWith(test, copier)
	if len(messages) != 2 || messages[0] != tea.SetClipboard("internal/sync/client.go")() {
		test.Errorf("msgs = %#v", messages)
	}
	if logged.Len() != 0 {
		test.Errorf("logged %q for a missing tool", logged.String())
	}
}

func TestCopyFilenameToolFailureIsLoggedNotShown(test *testing.T) {
	copier := &fakeCopier{err: errors.New("run xclip: exit status 1")}
	logged := captureLog(test)
	messages := copyWith(test, copier)
	if len(messages) != 2 {
		test.Fatalf("msgs = %#v", messages)
	}
	if info, ok := messages[1].(userinterface.InfoMessage); !ok || info.Text != "Copied internal/sync/client.go" {
		test.Errorf("status = %#v", messages[1])
	}
	if !strings.Contains(logged.String(), "exit status 1") || strings.Contains(logged.String(), "client.go") {
		test.Errorf("log = %q, want the error and not the text", logged.String())
	}
}

func TestCopyFilenameRefusedPathIsNotPiped(test *testing.T) {
	copier := &fakeCopier{}
	harness := loaded(test, 120, 30)
	WithClipboard(copier)(harness.view)
	harness.press("j")
	harness.view.detail.Files[0].Path = "a\nb.go"
	copyCommand(test, harness.view)
	if len(copier.texts) != 0 {
		test.Errorf("piped %q", copier.texts)
	}
}

// captureLog points the default logger at a buffer for the test. It changes a
// global, so the test using it must not run in parallel.
func captureLog(test *testing.T) *bytes.Buffer {
	test.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}
