package userinterface

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
type commandView struct {
	*fakeView
	pings []string
}

func (c *commandView) Update(msg tea.Msg) (View, tea.Cmd) {
	_, cmd := c.fakeView.Update(msg)
	return c, cmd
}

func (c *commandView) Command(name, args string) (tea.Cmd, bool) {
	if name != "review" {
		return nil, false
	}
	c.pings = append(c.pings, args)
	return Info("pong"), true
}

func TestCommanderReachesOnlyTopView(t *testing.T) {
	below := &commandView{fakeView: newFake("below")}
	top := &commandView{fakeView: newFake("top")}
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
		m := newModel(&commandView{fakeView: newFake("v")})
		run(m, typeCommand(m, "approve"))
		if m.status.text != "not available here: approve" || m.status.kind != statusError {
			t.Errorf("status = %+v", m.status)
		}
	})
	t.Run("a typo is not a command, whatever the view", func(t *testing.T) {
		m := newModel(&commandView{fakeView: newFake("v")})
		run(m, typeCommand(m, "aprove"))
		if m.status.text != "not a command: aprove" {
			t.Errorf("status = %q", m.status.text)
		}
	})
	t.Run("static commands win and still work", func(t *testing.T) {
		m := newModel(&commandView{fakeView: newFake("v")})
		if _, ok := typeCommand(m, "qa")().(QuitMessage); !ok {
			t.Error(":qa should still quit")
		}
	})
	t.Run("a view that is not a Commander", func(t *testing.T) {
		m := newModel(&commandView{fakeView: newFake("below")})
		run(m, Push(newFake("plain")))
		run(m, typeCommand(m, "merge squash"))
		if m.status.text != "not available here: merge" {
			t.Errorf("status = %q", m.status.text)
		}
	})
}

// copyView is a fake view that owns the command "copy-filename".
type copyView struct {
	*fakeView
	calls int
}

func (c *copyView) Update(msg tea.Msg) (View, tea.Cmd) {
	_, cmd := c.fakeView.Update(msg)
	return c, cmd
}

func (c *copyView) Command(name, _ string) (tea.Cmd, bool) {
	if name != "copy-filename" {
		return nil, false
	}
	c.calls++
	return Info("copied"), true
}

func TestCopyFilenameRoutedThroughCommandLine(t *testing.T) {
	below := &copyView{fakeView: newFake("below")}
	top := &copyView{fakeView: newFake("top")}
	m := newModel(below)
	run(m, Push(top))

	run(m, typeCommand(m, "copy-filename"))
	if top.calls != 1 || below.calls != 0 {
		t.Errorf("calls: top %d, below %d", top.calls, below.calls)
	}
	if m.status.text != "copied" {
		t.Errorf("status = %q", m.status.text)
	}

	run(m, Push(newFake("plain")))
	run(m, typeCommand(m, "copy-filename"))
	if m.status.text != "not available here: copy-filename" || top.calls != 1 {
		t.Errorf("status = %q, top calls %d", m.status.text, top.calls)
	}
}
