package userinterface

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

// promptingView asks for a prompt when "/" reaches it, like the list views.
type promptingView struct {
	*fakeView
	ask bool
}

func newPrompting() *promptingView { return &promptingView{fakeView: newFake("v")} }

func (view *promptingView) Update(message tea.Msg) (View, tea.Cmd) {
	if pressed, ok := message.(keys.Message); ok && pressed.Sequence == "/" {
		view.ask = true
	}
	_, command := view.fakeView.Update(message)
	return view, command
}

func (view *promptingView) PromptRequest() (string, bool) {
	ask := view.ask
	view.ask = false
	return "/", ask
}

func (view *fakeView) promptResults() (out []PromptResultMessage) {
	for _, message := range view.got {
		if result, ok := message.(PromptResultMessage); ok {
			out = append(out, result)
		}
	}
	return out
}

func bottomStartsWith(model *Model, prefix string) bool {
	return strings.HasPrefix(ansi.Strip(model.bottomRow()), prefix)
}

func TestPromptReportsToTheTopView(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	press(model, "/")
	if !model.commandLine.CapturesKeys() {
		test.Fatal("the prompt does not capture keys")
	}
	if !bottomStartsWith(model, "/") {
		test.Errorf("bottom line = %q, want it to start with the prompt", ansi.Strip(model.bottomRow()))
	}
	press(model, "a")
	press(model, "b")
	press(model, "backspace")
	press(model, "enter")
	if model.commandLine.CapturesKeys() {
		test.Error("enter left the prompt open")
	}
	want := []PromptResultMessage{
		{Prefix: "/", Text: "a", State: PromptChanged},
		{Prefix: "/", Text: "ab", State: PromptChanged},
		{Prefix: "/", Text: "a", State: PromptChanged},
		{Prefix: "/", Text: "a", State: PromptSubmitted},
	}
	if diff := cmp.Diff(want, view.promptResults()); diff != "" {
		test.Errorf("results (-want +got):\n%s", diff)
	}
	if keyMessages := view.keyMessages(); len(keyMessages) != 1 {
		test.Errorf("typing reached the view as keys: %v", keyMessages)
	}
}

func TestPromptCancel(test *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{"esc", []string{"x", "esc"}},
		{"backspace on an empty line", []string{"backspace"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := newPrompting()
			model := newModel(view)
			press(model, "/")
			for _, pressed := range testCase.keys {
				press(model, pressed)
			}
			if model.commandLine.CapturesKeys() {
				test.Error("capture was not released")
			}
			results := view.promptResults()
			if last := results[len(results)-1]; last.State != PromptCancelled {
				test.Errorf("last result = %+v, want cancelled", last)
			}
		})
	}
}

func TestCommandLineWorksAfterAPrompt(test *testing.T) {
	model := newModel(newPrompting())
	press(model, "/")
	press(model, "esc")
	press(model, ":")
	if !bottomStartsWith(model, ":") {
		test.Errorf("bottom line = %q, want the : prompt back", ansi.Strip(model.bottomRow()))
	}
}

func TestPromptOpensInTheSameUpdateAsTheKey(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	press(model, "/")
	if !model.commandLine.CapturesKeys() {
		test.Fatal("the prompt is not open right after /")
	}
	press(model, "enter") // typed straight after /: must not reach the view as a key
	if keyMessages := view.keyMessages(); len(keyMessages) != 1 || keyMessages[0].Sequence != "/" {
		test.Errorf("the view got keys %v, want only /", keyMessages)
	}
	results := view.promptResults()
	if len(results) != 1 || results[0].State != PromptSubmitted {
		test.Errorf("results = %+v", results)
	}
}

func TestPasteIsReportedAsAChange(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	press(model, "/")
	model.Update(tea.PasteMsg{Content: "pasted words"})
	results := view.promptResults()
	if len(results) != 1 || results[0].Text != "pasted words" || results[0].State != PromptChanged {
		test.Errorf("results = %+v", results)
	}
}

func TestPromptRequestIsIgnoredWhileTheCommandLineIsOpen(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	press(model, ":")
	press(model, "q")
	view.ask = true
	model.withRequestedPrompt(nil)
	if model.commandLine.prompt != "" || model.commandLine.input.Prompt != ":" {
		test.Error("the request converted the open : line")
	}
	if model.commandLine.input.Value() != "q" {
		test.Errorf("typed text = %q", model.commandLine.input.Value())
	}
}
