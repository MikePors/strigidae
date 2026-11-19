package main

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/config"
)

func TestGlobalKeys(test *testing.T) {
	global, err := globalKeys("config.yml", []config.KeyOverride{
		{Action: "half-page-down", Keys: []string{"ctrl+j"}, Line: 3},
		{Action: "top", Keys: []string{"g g", "home"}, Line: 4},
	})
	if err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]string{"ctrl+j"}, global.HalfDown.Keys()); diff != "" {
		test.Errorf("half-page-down (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"g g", "home"}, global.Top.Keys()); diff != "" {
		test.Errorf("top (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"ctrl+u"}, global.HalfUp.Keys()); diff != "" {
		test.Errorf("an action left out changed (-want +got):\n%s", diff)
	}
}

func TestGlobalKeysWithoutOverrides(test *testing.T) {
	global, err := globalKeys("config.yml", nil)
	if err != nil || global != nil {
		test.Errorf("globalKeys with no overrides = %v, %v; want nil, nil", global, err)
	}
}

func TestGlobalKeysErrorsNameTheLine(test *testing.T) {
	tests := []struct {
		name     string
		override config.KeyOverride
		want     string
	}{
		{"unknown action", config.KeyOverride{Action: "dwn", Keys: []string{"x"}, Line: 5}, `config.yml:5: unknown action "dwn"`},
		{"bad key", config.KeyOverride{Action: "down", Keys: []string{"ctrl-d"}, Line: 7}, `config.yml:7: action "down": key "ctrl-d"`},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			_, err := globalKeys("config.yml", []config.KeyOverride{testCase.override})
			if err == nil || !strings.HasPrefix(err.Error(), testCase.want) {
				test.Errorf("error = %v, want it to start with %q", err, testCase.want)
			}
		})
	}
}
