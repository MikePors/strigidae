package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeCommand(m *Model, line string) tea.Cmd {
	press(m, ":")
	for _, r := range line {
		press(m, string(r))
	}
	return press(m, "enter")
}

// cmdView is a fake view that owns the command "review".
type cmdView struct {
	*fakeView
	pings []string
}

func (c *cmdView) Update(msg tea.Msg) (View, tea.Cmd) {
	_, cmd := c.fakeView.Update(msg)
	return c, cmd
}

func (c *cmdView) Command(name, args string) (tea.Cmd, bool) {
	if name != "review" {
		return nil, false
	}
	c.pings = append(c.pings, args)
	return Info("pong"), true
}

func TestCommanderReachesOnlyTopView(t *testing.T) {
	below := &cmdView{fakeView: newFake("below")}
	top := &cmdView{fakeView: newFake("top")}
	m := newModel(below)
	run(m, Push(top))

	run(m, typeCommand(m, "review now"))
	if len(top.pings) != 1 || top.pings[0] != "now" || len(below.pings) != 0 {
		t.Errorf("pings: top %v, below %v", top.pings, below.pings)
	}
	if m.status.text != "pong" {
		t.Errorf("status = %q", m.status.text)
	}
}

func TestCommanderDeclinedAndPlainViews(t *testing.T) {
	t.Run("commander declines", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("v")})
		run(m, typeCommand(m, "approve"))
		if m.status.text != "not available here: approve" || m.status.kind != statusError {
			t.Errorf("status = %+v", m.status)
		}
	})
	t.Run("a typo is not a command, whatever the view", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("v")})
		run(m, typeCommand(m, "aprove"))
		if m.status.text != "not a command: aprove" {
			t.Errorf("status = %q", m.status.text)
		}
	})
	t.Run("static commands win and still work", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("v")})
		if _, ok := typeCommand(m, "qa")().(QuitMsg); !ok {
			t.Error(":qa should still quit")
		}
	})
	t.Run("a view that is not a Commander", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("below")})
		run(m, Push(newFake("plain")))
		run(m, typeCommand(m, "review"))
		if m.status.text != "not available here: review" {
			t.Errorf("status = %q", m.status.text)
		}
	})
}
