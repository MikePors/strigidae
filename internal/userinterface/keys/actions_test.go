package keys

import (
	"reflect"
	"regexp"
	"testing"

	"charm.land/bubbles/v2/key"
)

var actionName = regexp.MustCompile(`^[a-z]+(-[a-z]+)*$`)

func TestGlobalActionNames(test *testing.T) {
	global := DefaultGlobal()
	seen := map[string]bool{}
	for _, action := range global.Actions() {
		if !actionName.MatchString(action.Name) {
			test.Errorf("action name %q is not lower-case words joined by hyphens", action.Name)
		}
		if seen[action.Name] {
			test.Errorf("action name %q is used twice", action.Name)
		}
		seen[action.Name] = true
	}
}

// Every binding field of Global except Count is named exactly once, so a
// binding added later can't be left without a name.
func TestGlobalActionsCoverEveryBinding(test *testing.T) {
	global := DefaultGlobal()
	named := map[uintptr]int{} // by address, so unexported fields need no Interface
	for _, action := range global.Actions() {
		named[reflect.ValueOf(action.Binding).Pointer()]++
	}
	value := reflect.ValueOf(&global).Elem()
	bindingType := reflect.TypeFor[key.Binding]()
	for i := range value.NumField() {
		field := value.Type().Field(i)
		if field.Type != bindingType || field.Name == "Count" {
			continue
		}
		binding := value.Field(i).Addr().Pointer()
		if named[binding] != 1 {
			test.Errorf("Global.%s is named %d times, want once", field.Name, named[binding])
		}
		delete(named, binding)
	}
	if len(named) != 0 {
		test.Errorf("%d actions point outside Global's binding fields", len(named))
	}
}

func TestGlobalActionChangesItsBinding(test *testing.T) {
	global := DefaultGlobal()
	for _, action := range global.Actions() {
		if action.Name == "down" {
			action.Binding.SetKeys("ctrl+n")
		}
	}
	if keys := global.Down.Keys(); len(keys) != 1 || keys[0] != "ctrl+n" {
		test.Errorf("Down keys = %v, want [ctrl+n]", keys)
	}
}
