package workspace

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/ui"
)

func copyCommand(t *testing.T, v *View) []tea.Msg {
	t.Helper()
	cmd, ok := v.Command("copy-filename", "")
	if !ok {
		t.Fatal("copy-filename not handled")
	}
	return messages(cmd)
}

func TestCopyFilename(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{"files pane", []string{"j"}},
		{"diff pane", []string{"j", "l"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := loaded(t, 120, 30)
			h.press(tc.keys...)
			path := h.v.selectedPath()
			if path == "" {
				t.Fatal("no file selected")
			}
			msgs := copyCommand(t, h.v)
			if len(msgs) != 2 || msgs[0] != tea.SetClipboard(path)() {
				t.Fatalf("msgs = %#v, want clipboard %q first", msgs, path)
			}
			if info, ok := msgs[1].(ui.InfoMsg); !ok || info.Text != "Copied "+path {
				t.Errorf("status = %#v", msgs[1])
			}
		})
	}
}

func TestCopyFilenameOverview(t *testing.T) {
	h := loaded(t, 120, 30)
	msgs := copyCommand(t, h.v)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v", msgs)
	}
	if e, ok := msgs[0].(ui.ErrorMsg); !ok || e.Err.Error() != "no file to copy" {
		t.Errorf("msg = %#v", msgs[0])
	}
}

func TestCopyFilenameRefusesControlCharacters(t *testing.T) {
	h := loaded(t, 120, 30)
	h.press("j")
	h.v.detail.Files[0].Path = "a\x1b]52;c;x\x07b.go"
	msgs := copyCommand(t, h.v)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v", msgs)
	}
	e, ok := msgs[0].(ui.ErrorMsg)
	if !ok || !strings.Contains(e.Err.Error(), "control characters") || strings.ContainsAny(e.Err.Error(), "\x1b\x07") {
		t.Errorf("msg = %#v", msgs[0])
	}
}
