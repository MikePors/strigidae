package userinterface

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

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
	view := newFake("v")
	model := newModel(view)
	run(model, Prompt("/"))
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
	if keyMessages := view.keyMessages(); len(keyMessages) != 0 {
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
			view := newFake("v")
			model := newModel(view)
			run(model, Prompt("/"))
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
	model := newModel(newFake("v"))
	run(model, Prompt("/"))
	press(model, "esc")
	press(model, ":")
	if !bottomStartsWith(model, ":") {
		test.Errorf("bottom line = %q, want the : prompt back", ansi.Strip(model.bottomRow()))
	}
}
