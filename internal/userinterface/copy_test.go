package userinterface

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
)

type fakeCopier struct {
	texts []string
	err   error
}

func (copier *fakeCopier) Copy(_ context.Context, text string) error {
	copier.texts = append(copier.texts, text)
	return copier.err
}

// copyMessages runs a copy command and returns what it produced, batches
// flattened and empty results left out.
func copyMessages(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		var all []tea.Msg
		for _, inner := range batch {
			all = append(all, copyMessages(inner)...)
		}
		return all
	}
	if message == nil {
		return nil
	}
	return []tea.Msg{message}
}

func TestCopyURL(test *testing.T) {
	const url = "https://example.test/acme/widgets/pull/7"
	copier := &fakeCopier{}
	messages := copyMessages(CopyURL(copier, Timeouts{}, url))
	if len(messages) != 2 || messages[0] != tea.SetClipboard(url)() {
		test.Fatalf("messages = %#v, want OSC 52 and the status", messages)
	}
	if diff := cmp.Diff(InfoMessage{Text: "Copied " + url}, messages[1]); diff != "" {
		test.Errorf("status (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{url}, copier.texts); diff != "" {
		test.Errorf("piped (-want +got):\n%s", diff)
	}
}

func TestCopyURLRefusals(test *testing.T) {
	tests := []struct {
		name    string
		url     string
		message string
	}{
		{"empty", "", "no pull request URL to copy"},
		{"escape sequence", "https://x/\x1b]52;c;x\x07y", "not copied: "},
		{"line break", "https://x/a\nb", "not copied: "},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			copier := &fakeCopier{}
			messages := copyMessages(CopyURL(copier, Timeouts{}, testCase.url))
			if len(messages) != 1 {
				test.Fatalf("messages = %#v", messages)
			}
			refusal, ok := messages[0].(ErrorMessage)
			if !ok || !strings.HasPrefix(refusal.Err.Error(), testCase.message) || strings.ContainsAny(refusal.Err.Error(), "\x1b\x07\n") {
				test.Errorf("message = %#v", messages[0])
			}
			if len(copier.texts) != 0 {
				test.Errorf("piped %q", copier.texts)
			}
		})
	}
}

func TestCopyTextWithoutToolOrCopier(test *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	test.Cleanup(func() { slog.SetDefault(previous) })

	for name, copier := range map[string]clipboard.Copier{
		"no copier":     nil,
		"no tool found": &fakeCopier{err: clipboard.ErrNoTool},
		"tool failure":  &fakeCopier{err: errors.New("run xclip: exit status 1")},
	} {
		test.Run(name, func(test *testing.T) {
			logged.Reset()
			messages := copyMessages(CopyText(copier, Timeouts{}, "text", "Copied text"))
			if len(messages) != 2 || messages[0] != tea.SetClipboard("text")() {
				test.Fatalf("messages = %#v, want OSC 52 and the status", messages)
			}
			if failed := strings.Contains(logged.String(), "exit status 1"); failed != (name == "tool failure") {
				test.Errorf("log = %q", logged.String())
			}
			if strings.Contains(logged.String(), "text") {
				test.Errorf("log %q holds the copied text", logged.String())
			}
		})
	}
}
