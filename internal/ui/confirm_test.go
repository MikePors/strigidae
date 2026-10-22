package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type answered struct{}

func askConfirm(m *Model, text string) {
	run(m, Confirm(text, func() tea.Msg { return answered{} }))
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		key     string
		wantYes bool
	}{
		{"y", true},
		{"esc", false},
		{"n", false},
		{"enter", false},
		{"Y", false},
		{"j", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			v := newFake("v")
			m := newModel(v)
			askConfirm(m, "Post? \"hi\"")
			if !m.confirm.CapturesKeys() {
				t.Fatal("confirm should capture keys")
			}
			cmd := press(m, tt.key)
			if m.confirm.CapturesKeys() {
				t.Error("capture not released")
			}
			if got := cmd != nil && isAnswered(cmd); got != tt.wantYes {
				t.Errorf("OnYes ran = %v, want %v", got, tt.wantYes)
			}
			if len(v.keyMsgs()) != 0 {
				t.Error("the key reached the view")
			}
		})
	}
}

func isAnswered(cmd tea.Cmd) bool {
	_, ok := cmd().(answered)
	return ok
}

func TestConfirmRendersAndFits(t *testing.T) {
	m := newModel(newFake("v"))
	askConfirm(m, "Post reply to a.go:3? \"hello\x1b[31m world\"")
	row := ansi.Strip(m.bottomRow())
	if !strings.HasPrefix(row, " Post reply to a.go:3?") || !strings.Contains(row, "y/N") {
		t.Errorf("row = %q", row)
	}
	if strings.ContainsRune(row, 0x1b) {
		t.Errorf("escape in %q", row)
	}
	long := strings.Repeat("x", 200)
	m.confirm = nil // a second question is dropped while one is open
	askConfirm(m, long)
	row = ansi.Strip(m.bottomRow())
	if ansi.StringWidth(row) != 60 || !strings.HasSuffix(strings.TrimRight(row, " "), "… y/N") {
		t.Errorf("long row = %q", row)
	}
}

func TestConfirmTabsKeepChoicesVisible(t *testing.T) {
	m := newModel(newFake("v"))
	askConfirm(m, "Post comment? \"foo\tbar"+strings.Repeat("\tx", 60)+"\"")
	row := m.bottomRow()
	if w := lipgloss.Width(row); w != 60 {
		t.Errorf("row is %d wide, want 60", w)
	}
	if !strings.HasSuffix(strings.TrimRight(ansi.Strip(row), " "), "y/N") {
		t.Errorf("choices clipped: %q", ansi.Strip(row))
	}
}

func TestConfirmReleasesOnEsc(t *testing.T) {
	m := newModel(newFake("v"))
	askConfirm(m, "q")
	press(m, "esc")
	if m.confirm.CapturesKeys() {
		t.Error("confirm still captures after esc")
	}
}
