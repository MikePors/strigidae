package userinterface

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func remappedKeys(test *testing.T) *keys.Global {
	test.Helper()
	global := keys.DefaultGlobal()
	if err := global.Override("down", []string{"w"}); err != nil {
		test.Fatal(err)
	}
	if err := global.Override("top", []string{"t t"}); err != nil {
		test.Fatal(err)
	}
	return &global
}

func newRemappedModel(test *testing.T, root View) *Model {
	model := New(root, theme.Default(), Options{Tick: instantTick, GlobalKeys: remappedKeys(test)})
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	return model
}

func keysMessagesOf(view *fakeView) (messages []KeysMessage) {
	for _, message := range view.got {
		if keysMessage, ok := message.(KeysMessage); ok {
			messages = append(messages, keysMessage)
		}
	}
	return messages
}

func TestRemappedKeysReachTheView(test *testing.T) {
	view := newFake("v")
	model := newRemappedModel(test, view)

	press(model, "w")
	press(model, "t")
	press(model, "t")
	press(model, "j") // the old key is no longer bound
	press(model, "g")
	press(model, "g")
	want := []keys.Message{{Sequence: "w"}, {Sequence: "t t"}}
	if diff := cmp.Diff(want, view.keyMessages()); diff != "" {
		test.Errorf("keys delivered (-want +got):\n%s", diff)
	}
}

func TestRemappedKeysAreShownInHelp(test *testing.T) {
	model := newRemappedModel(test, newFake("v"))
	help := ansi.Strip(strings.Join(model.helpLines(), "\n"))
	if !strings.Contains(help, "tt ") || !strings.Contains(help, "top") {
		test.Errorf("help does not show the remapped top key:\n%s", help)
	}
	if strings.Contains(help, "gg ") {
		test.Errorf("help still shows the default top key:\n%s", help)
	}
}

func TestViewsAreToldTheKeys(test *testing.T) {
	root := newFake("root")
	model := newRemappedModel(test, root)
	if got := keysMessagesOf(root); len(got) != 1 || got[0].Global.Down.Keys()[0] != "w" {
		test.Errorf("root view heard %v, want one KeysMessage with the remapped keys", got)
	}
	pushed := newFake("pushed")
	model.Update(PushMessage{View: pushed})
	if got := keysMessagesOf(pushed); len(got) != 1 || got[0].Global.Down.Keys()[0] != "w" {
		test.Errorf("pushed view heard %v, want one KeysMessage with the remapped keys", got)
	}
}

func TestDefaultKeysSendNoMessage(test *testing.T) {
	root := newFake("root")
	model := newModel(root)
	model.Update(PushMessage{View: newFake("pushed")})
	if got := keysMessagesOf(root); len(got) != 0 {
		test.Errorf("root view heard %v, want none when no keys are overridden", got)
	}
}

func TestStatusBarHintFollowsTheHelpKey(test *testing.T) {
	global := keys.DefaultGlobal()
	if err := global.Override("help", []string{"f5"}); err != nil {
		test.Fatal(err)
	}
	model := New(newFake("v"), theme.Default(), Options{Tick: instantTick, GlobalKeys: &global})
	if text := model.bar().text; !strings.HasPrefix(text, "f5 help") {
		test.Errorf("status bar hint = %q, want it to start with f5 help", text)
	}
}
