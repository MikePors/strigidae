package userinterface

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func typeCommand(model *Model, line string) tea.Cmd {
	press(model, ":")
	for _, character := range line {
		press(model, string(character))
	}
	return press(model, "enter")
}

// commandView is a fake view that owns the command "review".
type commandView struct {
	*fakeView
	pings []string
}

func (commandView *commandView) Update(message tea.Msg) (View, tea.Cmd) {
	_, command := commandView.fakeView.Update(message)
	return commandView, command
}

func (commandView *commandView) Command(name, arguments string) (tea.Cmd, bool) {
	if name != "review" {
		return nil, false
	}
	commandView.pings = append(commandView.pings, arguments)
	return Info("pong"), true
}

func TestCommanderReachesOnlyTopView(test *testing.T) {
	below := &commandView{fakeView: newFake("below")}
	top := &commandView{fakeView: newFake("top")}
	model := newModel(below)
	run(model, Push(top))

	run(model, typeCommand(model, "review now"))
	if len(top.pings) != 1 || top.pings[0] != "now" || len(below.pings) != 0 {
		test.Errorf("pings: top %v, below %v", top.pings, below.pings)
	}
	if model.status.text != "pong" {
		test.Errorf("status = %q", model.status.text)
	}
}

func TestCommanderDeclinedAndPlainViews(test *testing.T) {
	test.Run("commander declines", func(test *testing.T) {
		model := newModel(&commandView{fakeView: newFake("v")})
		run(model, typeCommand(model, "approve"))
		if model.status.text != "not available here: approve" || model.status.kind != statusError {
			test.Errorf("status = %+v", model.status)
		}
	})
	test.Run("a typo is not a command, whatever the view", func(test *testing.T) {
		model := newModel(&commandView{fakeView: newFake("v")})
		run(model, typeCommand(model, "aprove"))
		if model.status.text != "not a command: aprove" {
			test.Errorf("status = %q", model.status.text)
		}
	})
	test.Run("static commands win and still work", func(test *testing.T) {
		model := newModel(&commandView{fakeView: newFake("v")})
		if _, ok := typeCommand(model, "qa")().(QuitMessage); !ok {
			test.Error(":qa should still quit")
		}
	})
	test.Run("a view that is not a Commander", func(test *testing.T) {
		model := newModel(&commandView{fakeView: newFake("below")})
		run(model, Push(newFake("plain")))
		run(model, typeCommand(model, "merge squash"))
		if model.status.text != "not available here: merge" {
			test.Errorf("status = %q", model.status.text)
		}
	})
}

// copyView is a fake view that owns the command "copy-filename".
type copyView struct {
	*fakeView
	calls int
}

func (copyView *copyView) Update(message tea.Msg) (View, tea.Cmd) {
	_, command := copyView.fakeView.Update(message)
	return copyView, command
}

func (copyView *copyView) Command(name, _ string) (tea.Cmd, bool) {
	if name != "copy-filename" {
		return nil, false
	}
	copyView.calls++
	return Info("copied"), true
}

func TestCopyFilenameRoutedThroughCommandLine(test *testing.T) {
	below := &copyView{fakeView: newFake("below")}
	top := &copyView{fakeView: newFake("top")}
	model := newModel(below)
	run(model, Push(top))

	run(model, typeCommand(model, "copy-filename"))
	if top.calls != 1 || below.calls != 0 {
		test.Errorf("calls: top %d, below %d", top.calls, below.calls)
	}
	if model.status.text != "copied" {
		test.Errorf("status = %q", model.status.text)
	}

	run(model, Push(newFake("plain")))
	run(model, typeCommand(model, "copy-filename"))
	if model.status.text != "not available here: copy-filename" || top.calls != 1 {
		test.Errorf("status = %q, top calls %d", model.status.text, top.calls)
	}
}
