package config

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/options"
)

func TestParseOptions(test *testing.T) {
	for name, testCase := range map[string]struct {
		yaml string
		want options.Values
	}{
		"unset":     {"theme: x\n", nil},
		"wrapscan":  {"wrapscan: false\n", options.Values{"wrapscan": false}},
		"both":      {"wrapscan: true\nhlsearch: false\n", options.Values{"wrapscan": true, "hlsearch": false}},
		"with tabs": {"hlsearch: false\ntabs:\n  - name: Me\n", options.Values{"hlsearch": false}},
	} {
		test.Run(name, func(test *testing.T) {
			parsed, err := Parse("c.yml", []byte(testCase.yaml))
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, parsed.Options); diff != "" {
				test.Error(diff)
			}
		})
	}
}

func TestParseOptionErrors(test *testing.T) {
	for name, testCase := range map[string]struct{ yaml, want string }{
		"text":      {"wrapscan: maybe\n", "c.yml:1: wrapscan must be true or false"},
		"quoted":    {"theme: x\nhlsearch: 'true'\n", "c.yml:2: hlsearch must be true or false"},
		"number":    {"wrapscan: 1\n", "c.yml:1: wrapscan must be true or false"},
		"empty":     {"hlsearch:\n", "c.yml:1: hlsearch must be true or false"},
		"duplicate": {"wrapscan: true\nwrapscan: false\n", `c.yml:2: duplicate key "wrapscan" in the config file`},
	} {
		test.Run(name, func(test *testing.T) {
			_, err := Parse("c.yml", []byte(testCase.yaml))
			if err == nil {
				test.Fatal("no error")
			}
			if diff := cmp.Diff(testCase.want, err.Error()); diff != "" {
				test.Error(diff)
			}
		})
	}
}

// TestEveryDeclaredOptionIsAConfigKey keeps the config in step with the
// declarations in package options.
func TestEveryDeclaredOptionIsAConfigKey(test *testing.T) {
	for _, option := range options.All() {
		for _, value := range []bool{true, false} {
			parsed, err := Parse("c.yml", []byte(option.Name+": "+strconv.FormatBool(value)+"\n"))
			if err != nil {
				test.Errorf("%s: %v", option.Name, err)
				continue
			}
			if parsed.Options.Get(option.Name) != value {
				test.Errorf("%s: parsed %v, want %v", option.Name, !value, value)
			}
		}
	}
}
