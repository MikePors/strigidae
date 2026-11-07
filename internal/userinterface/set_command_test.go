package userinterface

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/options"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestApplySet(test *testing.T) {
	tests := []struct {
		name      string
		start     options.Values
		arguments string
		want      options.Values
		wantText  string
		wantError string
	}{
		{name: "bare with nothing changed", arguments: "", wantText: "no options changed"},
		{name: "bare lists only what differs from the default", start: options.Values{"wrapscan": false, "wrap": true}, arguments: "  ", want: options.Values{"wrapscan": false, "wrap": true}, wantText: "nowrapscan wrap"},
		{name: "bare skips a value set to its default", start: options.Values{"wrapscan": true, "hlsearch": false}, arguments: "", want: options.Values{"wrapscan": true, "hlsearch": false}, wantText: "nohlsearch"},
		{name: "bare after setting back to the default", start: options.Values{"wrapscan": true}, arguments: "", want: options.Values{"wrapscan": true}, wantText: "no options changed"},
		{name: "all lists every option", arguments: "all", wantText: "wrapscan hlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber"},
		{name: "all shows the current values", start: options.Values{"wrapscan": false}, arguments: " all ", want: options.Values{"wrapscan": false}, wantText: "nowrapscan hlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber"},
		{name: "all can't be mixed with options", arguments: "all wrap", wantError: "unknown option: all"},
		{name: "turn off", arguments: "nowrapscan", want: options.Values{"wrapscan": false}, wantText: "nowrapscan"},
		{name: "turn on", start: options.Values{"hlsearch": false}, arguments: "hlsearch", want: options.Values{"hlsearch": true}, wantText: "hlsearch"},
		{name: "several", arguments: "nowrapscan nohlsearch", want: options.Values{"wrapscan": false, "hlsearch": false}, wantText: "nowrapscan nohlsearch"},
		{name: "extra spaces", arguments: " nowrapscan   hlsearch ", want: options.Values{"wrapscan": false, "hlsearch": true}, wantText: "nowrapscan hlsearch"},
		{name: "invalid form cleans terminal escapes", arguments: "inv\x1b[31m?", wantError: "invalid :set form: inv [31m?"},
		{name: "query on", arguments: "wrapscan?", wantText: "wrapscan"},
		{name: "query off", start: options.Values{"wrapscan": false}, arguments: "wrapscan?", want: options.Values{"wrapscan": false}, wantText: "nowrapscan"},
		{name: "query changes nothing", arguments: "hlsearch?", wantText: "hlsearch"},
		{name: "unknown", arguments: "foo", wantError: "unknown option: foo"},
		{name: "unknown with no", arguments: "nofoo", wantError: "unknown option: nofoo"},
		{name: "unknown query", arguments: "foo?", wantError: "unknown option: foo?"},
		{name: "value form", arguments: "wrapscan=false", wantError: "wrapscan is on or off: use :set wrapscan or :set nowrapscan"},
		{name: "value form with no", arguments: "nowrapscan=1", wantError: "wrapscan is on or off: use :set wrapscan or :set nowrapscan"},
		{name: "query with no name", arguments: "?", wantError: "missing option name"},
		{name: "query mixed with a change", arguments: "wrapscan? nowrapscan", wantError: "can't mix name? with changes in one :set line"},
		{name: "unknown value form", arguments: "foo=1", wantError: "unknown option: foo=1"},
		{name: "a bad word changes nothing, even after a good one", arguments: "nowrapscan foo", wantError: "unknown option: foo"},
		{name: "escapes in the name are cleaned", arguments: "foo\x1b[31m", wantError: "unknown option: foo [31m"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, text, _, err := applySet(testCase.arguments, testCase.start)
			if testCase.wantError != "" {
				if err == nil || err.Error() != testCase.wantError {
					test.Fatalf("error = %v, want %q", err, testCase.wantError)
				}
				if diff := cmp.Diff(testCase.start, got); diff != "" {
					test.Errorf("an error changed the values (-want +got):\n%s", diff)
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("values (-want +got):\n%s", diff)
			}
			if text != testCase.wantText {
				test.Errorf("text = %q, want %q", text, testCase.wantText)
			}
		})
	}
}

// TestEveryDeclaredOptionWorksWithSet walks the declarations, so a new option
// is accepted by `:set` without further code.
func TestEveryDeclaredOptionWorksWithSet(test *testing.T) {
	for _, option := range options.All() {
		for _, arguments := range []string{option.Name, "no" + option.Name, option.Name + "?"} {
			if _, _, _, err := applySet(arguments, nil); err != nil {
				test.Errorf(":set %s: %v", arguments, err)
			}
		}
	}
}

func setModel(root View, values options.Values) *Model {
	model := New(root, theme.Default(), Options{Tick: neverTick, StartingOptions: values})
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	return model
}

func optionsSeen(view *fakeView) []options.Values {
	var seen []options.Values
	for _, message := range view.got {
		if optionsMessage, ok := message.(OptionsMessage); ok {
			seen = append(seen, optionsMessage.Values)
		}
	}
	return seen
}

func TestSetCommandChangesOptionsForEveryView(test *testing.T) {
	below, top := newFake("below"), newFake("top")
	model := setModel(below, nil)
	drive(model, Push(top))

	drive(model, parseCommand("set nowrapscan", model.top(), model.viewCommands))

	want := []options.Values{{"wrapscan": false}}
	for name, view := range map[string]*fakeView{"below": below, "top": top} {
		if diff := cmp.Diff(want, optionsSeen(view)); diff != "" {
			test.Errorf("%s view's options messages (-want +got):\n%s", name, diff)
		}
	}
	if model.status.text != "nowrapscan" || model.status.kind != statusInfo {
		test.Errorf("status = %+v, want the new value", model.status)
	}

	// Replace the status first, so a bare :set that did nothing would fail.
	drive(model, Info("stale"))
	drive(model, parseCommand("set", model.top(), model.viewCommands))
	if model.status.text != "nowrapscan" || model.status.kind != statusInfo {
		test.Errorf("bare :set shows %+v", model.status)
	}
	drive(model, Info("stale"))
	drive(model, parseCommand("set all", model.top(), model.viewCommands))
	if model.status.text != "nowrapscan hlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber" {
		test.Errorf(":set all shows %q", model.status.text)
	}
	if got := len(optionsSeen(below)); got != 1 {
		test.Errorf(":set and :set all sent a message; %d in all, want 1", got)
	}
}

func TestBareSetWithNothingChanged(test *testing.T) {
	model := setModel(newFake("v"), nil)
	drive(model, Info("stale"))
	drive(model, parseCommand("set", model.top(), model.viewCommands))
	if model.status.text != "no options changed" || model.status.kind != statusInfo {
		test.Errorf("bare :set with the defaults shows %+v", model.status)
	}
}

func TestSetErrorChangesNothing(test *testing.T) {
	root := newFake("v")
	model := setModel(root, nil)
	drive(model, parseCommand("set foo", model.top(), model.viewCommands))
	if model.status.text != "unknown option: foo" || model.status.kind != statusError {
		test.Errorf("status = %+v", model.status)
	}
	if len(optionsSeen(root)) != 0 || model.options != nil {
		test.Error("an error changed the options")
	}
}

func TestOptionsReachTheRootViewAtStartAndPushedViews(test *testing.T) {
	root := newFake("root")
	values := options.Values{"hlsearch": false}
	model := setModel(root, values)
	if diff := cmp.Diff([]options.Values{values}, optionsSeen(root)); diff != "" {
		test.Errorf("root view at start (-want +got):\n%s", diff)
	}

	pushed := newFake("pushed")
	drive(model, Push(pushed))
	if diff := cmp.Diff([]options.Values{values}, optionsSeen(pushed)); diff != "" {
		test.Errorf("pushed view (-want +got):\n%s", diff)
	}

	// With nothing set, views keep their defaults and hear nothing.
	plain := newFake("plain")
	setModel(plain, nil)
	if len(optionsSeen(plain)) != 0 {
		test.Error("a view was told about options that were never set")
	}
}

func TestSetWrapReachesViews(test *testing.T) {
	root := newFake("v")
	model := setModel(root, nil)
	drive(model, parseCommand("set wrap", model.top(), model.viewCommands))
	drive(model, parseCommand("set nowrap", model.top(), model.viewCommands))
	want := []options.Values{{"wrap": true}, {"wrap": false}}
	if diff := cmp.Diff(want, optionsSeen(root)); diff != "" {
		test.Errorf("options messages (-want +got):\n%s", diff)
	}
}

func TestSetIsAStaticCommandInEveryView(test *testing.T) {
	model := setModel(newFake("not a commander"), nil)
	drive(model, parseCommand("set hlsearch?", model.top(), model.viewCommands))
	if model.status.text != "hlsearch" {
		test.Errorf("status = %q", model.status.text)
	}
}

func TestSetToggleAndResetThroughModel(test *testing.T) {
	for _, testCase := range []struct {
		arguments string
		want      options.Values
		wantError string
		wantText  string
	}{
		{arguments: "wrap!", want: options.Values{"wrap": false, "wrapscan": false}, wantText: "nowrap"},
		{arguments: "invwrapscan", want: options.Values{"wrap": true, "wrapscan": true}, wantText: "wrapscan"},
		{arguments: "wrap& wrapscan&", want: options.Values{"wrap": false, "wrapscan": true}, wantText: "nowrap wrapscan"},
		{arguments: "nowrap wrap! invwrap wrapscan& nowrapscan", want: options.Values{"wrap": false, "wrapscan": false}, wantText: "nowrap nowrap nowrap nowrapscan nowrapscan"},
		{arguments: "wrap! wrap!", want: options.Values{"wrap": true, "wrapscan": false}, wantText: "wrap wrap"},
		{arguments: "wrap& wrap&", want: options.Values{"wrap": false, "wrapscan": false}, wantText: "nowrap nowrap"},
		{arguments: "wrapscan& wrapscan&", want: options.Values{"wrap": true, "wrapscan": true}, wantText: "wrapscan wrapscan"},
		{arguments: "wrap! invwrap!", wantError: "invalid :set form: invwrap!"},
		{arguments: "wrap! invinvwrap", wantError: "invalid :set form: invinvwrap"},
		{arguments: "wrap! inv\x1b[31m?", wantError: "invalid :set form: inv [31m?"},
		{arguments: "wrap! invisible", wantError: "unknown option: invisible"},
		{arguments: "wrap! wrap&?", wantError: "unknown option: wrap&?"},
		{arguments: "wrap! wrap!?", wantError: "unknown option: wrap!?"},
		{arguments: "wrap! wrap!=1", wantError: "unknown option: wrap!=1"},
		{arguments: "wrap! noinvwrap", wantError: "unknown option: noinvwrap"},
		{arguments: "!", wantError: "missing option name"},
		{arguments: "&", wantError: "missing option name"},
		{arguments: "inv", wantError: "missing option name"},
		{arguments: "wrap! nowrap!", wantError: "invalid :set form: nowrap!"},
		{arguments: "wrap! invnowrap", wantError: "invalid :set form: invnowrap"},
		{arguments: "wrap! invwrap?", wantError: "invalid :set form: invwrap?"},
		{arguments: "wrap! invwrap=1", wantError: "invalid :set form: invwrap=1"},
		{arguments: "wrap! unknown!", wantError: "unknown option: unknown!"},
		{arguments: "wrap& invunknown", wantError: "unknown option: invunknown"},
		{arguments: "invwrap unknown&", wantError: "unknown option: unknown&"},
		{arguments: "wrap! wrapscan?", wantError: "can't mix name? with changes in one :set line"},
	} {
		test.Run(testCase.arguments, func(test *testing.T) {
			start := options.Values{"wrap": true, "wrapscan": false}
			root := newFake("root")
			model := setModel(root, start)
			root.got = nil
			drive(model, parseCommand("set "+testCase.arguments, model.top(), model.viewCommands))
			want := testCase.want
			if testCase.wantError != "" {
				want = start
				if model.status.kind != statusError || model.status.text != testCase.wantError {
					test.Errorf("status = %+v, want %q", model.status, testCase.wantError)
				}
				if len(optionsSeen(root)) != 0 {
					test.Error("failed line broadcast options")
				}
			} else if diff := cmp.Diff([]options.Values{want}, optionsSeen(root)); diff != "" {
				test.Errorf("broadcast (-want +got):\n%s", diff)
			}
			if testCase.wantError == "" && (model.status.kind != statusInfo || model.status.text != testCase.wantText) {
				test.Errorf("status = %+v, want %q", model.status, testCase.wantText)
			}
			if diff := cmp.Diff(want, model.options); diff != "" {
				test.Errorf("options (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(options.Values{"wrap": true, "wrapscan": false}, start); diff != "" {
				test.Errorf("starting config mutated: %s", diff)
			}
		})
	}
}
