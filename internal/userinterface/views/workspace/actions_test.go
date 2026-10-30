package workspace

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/MikePors/strigidae/internal/userinterface/keys"
)

func TestActionNames(test *testing.T) {
	global := keys.DefaultGlobal()
	workspaceBindings, paneBindings := defaultBindings(), defaultDiffBindings()
	actions := append(global.Actions(), workspaceBindings.actions()...)
	actions = append(actions, paneBindings.actions()...)
	if err := keys.CheckActions(actions); err != nil {
		test.Error(err)
	}
}

// Every binding of a bindings struct is named exactly once, so one added
// later can't be left without a name.
func TestActionsCoverEveryBinding(test *testing.T) {
	workspaceBindings, paneBindings := defaultBindings(), defaultDiffBindings()
	tests := []struct {
		name     string
		bindings any
		actions  []keys.Action
	}{
		{"workspace", &workspaceBindings, workspaceBindings.actions()},
		{"diff pane", &paneBindings, paneBindings.actions()},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			named := map[uintptr]int{}
			for _, action := range testCase.actions {
				if action.Binding != nil { // a command without keys
					named[reflect.ValueOf(action.Binding).Pointer()]++
				}
			}
			value := reflect.ValueOf(testCase.bindings).Elem()
			for i := range value.NumField() {
				field := value.Type().Field(i)
				if field.Type != reflect.TypeFor[key.Binding]() {
					continue
				}
				binding := value.Field(i).Addr().Pointer()
				if named[binding] != 1 {
					test.Errorf("%s is named %d times, want once", field.Name, named[binding])
				}
				delete(named, binding)
			}
			if len(named) != 0 {
				test.Errorf("%d actions point outside the bindings struct", len(named))
			}
		})
	}
}
