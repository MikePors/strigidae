package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type wrong struct{}

// TestLateConfirmIsSkipped pins that a question arriving while the user is
// busy is dropped: the existing input or prompt stays and a stray y does not
// run the action.
func TestLateConfirmIsSkipped(t *testing.T) {
	late := func(notice bool) tea.Cmd {
		return msgCmd(ConfirmMsg{Text: "late?", Notice: notice, OnYes: func() tea.Msg { return wrong{} }, OnDismiss: func() tea.Msg { return wrong{} }})
	}
	for _, notice := range []bool{false, true} {
		t.Run("command line", func(t *testing.T) {
			m := newModel(newFake("v"))
			press(m, ":")
			press(m, "c")
			run(m, late(notice))
			if m.confirm != nil || !m.cmdline.active {
				t.Fatalf("confirm = %v, cmdline active = %v", m.confirm, m.cmdline.active)
			}
			if m.status.text != confirmSkipped {
				t.Errorf("status = %q", m.status.text)
			}
			if cmd := press(m, "y"); cmd != nil && isWrong(cmd) {
				t.Error("y ran the skipped action")
			}
			if got := m.cmdline.input.Value(); got != "cy" {
				t.Errorf("command line = %q, want the y typed into it", got)
			}
		})
		t.Run("capturing view", func(t *testing.T) {
			v := newFake("v")
			v.capture = true
			m := newModel(v)
			run(m, late(notice))
			if m.confirm != nil || m.status.text != confirmSkipped {
				t.Errorf("confirm = %v, status = %q", m.confirm, m.status.text)
			}
			if cmd := press(m, "y"); cmd != nil && isWrong(cmd) {
				t.Error("y ran the skipped action")
			}
		})
	}
	t.Run("second prompt while one is open", func(t *testing.T) {
		m := newModel(newFake("v"))
		run(m, msgCmd(ConfirmMsg{Text: "first?", OnYes: func() tea.Msg { return answered{} }}))
		run(m, late(false))
		if row := ansi.Strip(m.bottomRow()); !strings.Contains(row, "first?") {
			t.Errorf("row = %q", row)
		}
		if cmd := press(m, "y"); cmd == nil || !isAnswered(cmd) {
			t.Error("the first prompt's action did not run")
		}
	})
}

func isWrong(cmd tea.Cmd) bool {
	_, ok := cmd().(wrong)
	return ok
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
