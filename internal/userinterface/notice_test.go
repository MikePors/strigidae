package userinterface

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type wrong struct{}

// TestConfirmReplacesOpenPrompt pins the documented behaviour: the newest
// question wins, and the replaced one's action can no longer run.
func TestConfirmReplacesOpenPrompt(test *testing.T) {
	model := newModel(newFake("v"))
	run(model, messageCommand(ConfirmMessage{Text: "first?", OnYes: func() tea.Msg { return wrong{} }}))
	run(model, messageCommand(ConfirmMessage{Text: "second?", OnYes: func() tea.Msg { return answered{} }}))
	if row := ansi.Strip(model.bottomRow()); !strings.Contains(row, "second?") {
		test.Errorf("row = %q", row)
	}
	if command := press(model, "y"); command == nil || !isAnswered(command) {
		test.Error("the replacing prompt's action did not run")
	}
}

func TestNoticeAnyKeyDismissesAndRuns(test *testing.T) {
	for _, k := range []string{"y", "n", "esc", "enter", "j"} {
		test.Run(k, func(test *testing.T) {
			model := newModel(newFake("v"))
			run(model, messageCommand(ConfirmMessage{Text: "Head moved.", OnDismiss: func() tea.Msg { return answered{} }, OnYes: func() tea.Msg { return wrong{} }, Notice: true}))
			row := ansi.Strip(model.bottomRow())
			if strings.Contains(row, "y/N") || !strings.Contains(row, "any key") {
				test.Errorf("row = %q", row)
			}
			command := press(model, k)
			if model.confirm.CapturesKeys() {
				test.Error("notice still captures")
			}
			if command == nil || !isAnswered(command) {
				test.Error("OnDismiss did not run")
			}
		})
	}
}
