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

// cmdView is a fake view that owns the command "ping".
type cmdView struct {
	*fakeView
	pings []string
}

func (c *cmdView) Update(msg tea.Msg) (View, tea.Cmd) {
	_, cmd := c.fakeView.Update(msg)
	return c, cmd
}

func (c *cmdView) Command(name, args string) (tea.Cmd, bool) {
	if name != "ping" {
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

	run(m, typeCommand(m, "ping now"))
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
		run(m, typeCommand(m, "other"))
		if m.status.text != "not available here: other" || m.status.kind != statusError {
			t.Errorf("status = %+v", m.status)
		}
	})
	t.Run("static commands win and still work", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("v")})
		if _, ok := typeCommand(m, "qa")().(QuitMsg); !ok {
			t.Error(":qa should still quit")
		}
	})
	t.Run("view below does not matter", func(t *testing.T) {
		m := newModel(&cmdView{fakeView: newFake("below")})
		run(m, Push(newFake("plain")))
		run(m, typeCommand(m, "ping"))
		if m.status.text != "not a command: ping" {
			t.Errorf("status = %q", m.status.text)
		}
	})
}
