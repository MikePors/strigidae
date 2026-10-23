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
func TestConfirmReplacesOpenPrompt(t *testing.T) {
	m := newModel(newFake("v"))
	run(m, msgCmd(ConfirmMsg{Text: "first?", OnYes: func() tea.Msg { return wrong{} }}))
	run(m, msgCmd(ConfirmMsg{Text: "second?", OnYes: func() tea.Msg { return answered{} }}))
	if row := ansi.Strip(m.bottomRow()); !strings.Contains(row, "second?") {
		t.Errorf("row = %q", row)
	}
	if cmd := press(m, "y"); cmd == nil || !isAnswered(cmd) {
		t.Error("the replacing prompt's action did not run")
	}
}

func TestNoticeAnyKeyDismissesAndRuns(t *testing.T) {
	for _, k := range []string{"y", "n", "esc", "enter", "j"} {
		t.Run(k, func(t *testing.T) {
			m := newModel(newFake("v"))
			run(m, msgCmd(ConfirmMsg{Text: "Head moved.", OnDismiss: func() tea.Msg { return answered{} }, OnYes: func() tea.Msg { return wrong{} }, Notice: true}))
			row := ansi.Strip(m.bottomRow())
			if strings.Contains(row, "y/N") || !strings.Contains(row, "any key") {
				t.Errorf("row = %q", row)
			}
			cmd := press(m, k)
			if m.confirm.CapturesKeys() {
				t.Error("notice still captures")
			}
			if cmd == nil || !isAnswered(cmd) {
				t.Error("OnDismiss did not run")
			}
		})
	}
}
