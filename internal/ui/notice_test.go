package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNoticeAnyKeyDismissesAndRuns(t *testing.T) {
	for _, k := range []string{"y", "n", "esc", "enter", "j"} {
		t.Run(k, func(t *testing.T) {
			m := newModel(newFake("v"))
			run(m, msgCmd(ConfirmMsg{Text: "Head moved.", OnYes: func() tea.Msg { return answered{} }, Notice: true}))
			row := ansi.Strip(m.bottomRow())
			if strings.Contains(row, "y/N") || !strings.Contains(row, "any key") {
				t.Errorf("row = %q", row)
			}
			cmd := press(m, k)
			if m.confirm.CapturesKeys() {
				t.Error("notice still captures")
			}
			if cmd == nil || !isAnswered(cmd) {
				t.Error("OnYes did not run")
			}
		})
	}
}
