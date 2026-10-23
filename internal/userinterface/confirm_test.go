package userinterface

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type answered struct{}

func askConfirm(model *Model, text string) {
	run(model, Confirm(text, func() tea.Msg { return answered{} }))
}

func TestConfirm(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.key, func(test *testing.T) {
			view := newFake("v")
			model := newModel(view)
			askConfirm(model, "Post? \"hi\"")
			if !model.confirm.CapturesKeys() {
				test.Fatal("confirm should capture keys")
			}
			command := press(model, testCase.key)
			if model.confirm.CapturesKeys() {
				test.Error("capture not released")
			}
			if got := command != nil && isAnswered(command); got != testCase.wantYes {
				test.Errorf("OnYes ran = %v, want %v", got, testCase.wantYes)
			}
			if len(view.keyMessages()) != 0 {
				test.Error("the key reached the view")
			}
		})
	}
}

func isAnswered(command tea.Cmd) bool {
	_, ok := command().(answered)
	return ok
}

func TestConfirmRendersAndFits(test *testing.T) {
	model := newModel(newFake("v"))
	askConfirm(model, "Post reply to a.go:3? \"hello\x1b[31m world\"")
	row := ansi.Strip(model.bottomRow())
	if !strings.HasPrefix(row, " Post reply to a.go:3?") || !strings.Contains(row, "y/N") {
		test.Errorf("row = %q", row)
	}
	if strings.ContainsRune(row, 0x1b) {
		test.Errorf("escape in %q", row)
	}
	long := strings.Repeat("x", 200)
	askConfirm(model, long)
	row = ansi.Strip(model.bottomRow())
	if ansi.StringWidth(row) != 60 || !strings.HasSuffix(strings.TrimRight(row, " "), "… y/N") {
		test.Errorf("long row = %q", row)
	}
}

func TestConfirmTabsKeepChoicesVisible(test *testing.T) {
	model := newModel(newFake("v"))
	askConfirm(model, "Post comment? \"foo\tbar"+strings.Repeat("\tx", 60)+"\"")
	row := model.bottomRow()
	if width := lipgloss.Width(row); width != 60 {
		test.Errorf("row is %d wide, want 60", width)
	}
	if !strings.HasSuffix(strings.TrimRight(ansi.Strip(row), " "), "y/N") {
		test.Errorf("choices clipped: %q", ansi.Strip(row))
	}
}

func TestConfirmReleasesOnEsc(test *testing.T) {
	model := newModel(newFake("v"))
	askConfirm(model, "q")
	press(model, "esc")
	if model.confirm.CapturesKeys() {
		test.Error("confirm still captures after esc")
	}
}
