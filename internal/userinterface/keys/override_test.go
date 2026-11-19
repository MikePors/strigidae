package keys

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/google/go-cmp/cmp"
)

func TestCheckKeys(test *testing.T) {
	tests := []struct {
		name      string
		sequences []string
		valid     bool
	}{
		{"letter", []string{"x"}, true},
		{"capital letter", []string{"G"}, true},
		{"punctuation", []string{"}", ":", "?"}, true},
		{"control key", []string{"ctrl+d"}, true},
		{"alt key", []string{"alt+x"}, true},
		{"named key", []string{"enter", "pgdown", "f5"}, true},
		{"sequence", []string{"g g"}, true},
		{"digit after the first key", []string{"g 1"}, true},
		{"no keys", nil, false},
		{"empty key", []string{""}, false},
		{"ctrl with a dash", []string{"ctrl-d"}, false},
		{"upper case after ctrl", []string{"ctrl+D"}, false},
		{"unknown name", []string{"escape"}, false},
		{"leading 1 to 9 is a count", []string{"1"}, false},
		{"leading 0 is a key", []string{"0"}, true},
		{"shift before a named key", []string{"shift+tab", "ctrl+shift+left"}, true},
		{"shift before a character", []string{"shift+g"}, false},
		{"ctrl+i is tab", []string{"ctrl+i"}, false},
		{"ctrl+m is enter", []string{"ctrl+m"}, false},
		{"ctrl+[ is esc", []string{"ctrl+["}, false},
		{"other control keys", []string{"ctrl+j", "ctrl+h"}, true},
		{"literal space", []string{" "}, false},
		{"double space", []string{"g  g"}, false},
		{"trailing space", []string{"g "}, false},
		{"one bad entry among good ones", []string{"j", "ctrl-d"}, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if err := CheckKeys(testCase.sequences); (err == nil) != testCase.valid {
				test.Errorf("CheckKeys(%q) = %v, want valid %v", testCase.sequences, err, testCase.valid)
			}
		})
	}
}

func TestOverride(test *testing.T) {
	global := DefaultGlobal()
	if err := global.Override("top", []string{"g g", "home"}); err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]string{"g g", "home"}, global.Top.Keys()); diff != "" {
		test.Errorf("keys (-want +got):\n%s", diff)
	}
	if got := global.Top.Help(); got.Key != "gg/home" || got.Desc != "top" {
		test.Errorf("help = %+v, want key gg/home and the description kept", got)
	}
	if diff := cmp.Diff(DefaultGlobal().Bottom.Keys(), global.Bottom.Keys()); diff != "" {
		test.Errorf("an action that was not named changed (-want +got):\n%s", diff)
	}
}

func TestOverrideErrors(test *testing.T) {
	global := DefaultGlobal()
	err := global.Override("dwn", []string{"x"})
	if err == nil || !strings.Contains(err.Error(), `unknown action "dwn"`) || !strings.Contains(err.Error(), "half-page-down") {
		test.Errorf("unknown action error = %v, want the name and the valid actions", err)
	}
	if err := global.Override("down", []string{"ctrl-d"}); err == nil || !strings.Contains(err.Error(), `action "down"`) {
		test.Errorf("invalid key error = %v, want it to name the action", err)
	}
	if diff := cmp.Diff(DefaultGlobal().Down.Keys(), global.Down.Keys()); diff != "" {
		test.Errorf("a refused override changed the keys (-want +got):\n%s", diff)
	}
}

func TestBindingsFollowOverrides(test *testing.T) {
	global := DefaultGlobal()
	// tab is also the help key of Complete, which Bindings must still leave out.
	if err := global.Override("open-in-browser", []string{"tab"}); err != nil {
		test.Fatal(err)
	}
	var all []string
	for _, binding := range global.Bindings() {
		all = append(all, binding.Keys()...)
	}
	if !slices.Contains(all, "tab") || slices.Contains(all, "o") {
		test.Errorf("bindings = %q, want tab and not o", all)
	}
	if global.Complete.Keys() == nil || slices.ContainsFunc(global.Bindings(), func(binding key.Binding) bool { return binding.Help().Desc == global.Complete.Help().Desc }) {
		test.Error("Complete is documentation only and must stay out of the bindings")
	}
}

func TestOverrideSingleKeyActions(test *testing.T) {
	for _, name := range []string{"help", "cancel", "command-line"} {
		global := DefaultGlobal()
		if err := global.Override(name, []string{"x x"}); err == nil || !strings.Contains(err.Error(), "single key") {
			test.Errorf("Override(%q, sequence) = %v, want a single-key error", name, err)
		}
		if err := global.Override(name, []string{"f5", "ctrl+j"}); err != nil {
			test.Errorf("Override(%q, single keys) = %v", name, err)
		}
	}
	global := DefaultGlobal()
	if err := global.Override("top", []string{"x x"}); err != nil {
		test.Errorf("a movement action may be a sequence: %v", err)
	}
}
