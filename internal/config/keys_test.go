package config

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseKeys(test *testing.T) {
	tests := []struct {
		name string
		yaml string
		want []KeyOverride
	}{
		{"one key", "keys:\n  down: n\n", []KeyOverride{{Action: "down", Keys: []string{"n"}, Line: 2}}},
		{"a sequence is one key", "keys:\n  top: g g\n", []KeyOverride{{Action: "top", Keys: []string{"g g"}, Line: 2}}},
		{"a list", "keys:\n  top: [g g, home]\n", []KeyOverride{{Action: "top", Keys: []string{"g g", "home"}, Line: 2}}},
		{"quoted punctuation", "keys:\n  command-line: ';'\n  help: '?'\n", []KeyOverride{
			{Action: "command-line", Keys: []string{";"}, Line: 2},
			{Action: "help", Keys: []string{"?"}, Line: 3},
		}},
		{"names are kept for the program to check", "keys:\n  nonsense: x\n", []KeyOverride{{Action: "nonsense", Keys: []string{"x"}, Line: 2}}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			configuration, err := Parse("config.yml", []byte(testCase.yaml))
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, configuration.Keys); diff != "" {
				test.Errorf("Keys (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseKeysErrors(test *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"not a mapping", "keys: [j]\n", "config.yml:1: keys must be a mapping"},
		{"empty", "keys: {}\n", "config.yml:1: keys must be a mapping"},
		{"duplicate action", "keys:\n  down: j\n  down: n\n", `config.yml:3: duplicate action "down" in keys`},
		{"empty list", "keys:\n  down: []\n", `config.yml:2: "down" needs at least one key`},
		{"empty value", "keys:\n  down: \"\"\n", `config.yml:2: a key of "down" must be a non-empty text value`},
		{"null value", "keys:\n  down:\n", `config.yml:2: a key of "down" must be a non-empty text value`},
		{"action name that is not text", "keys:\n  [a, b]: x\n", "config.yml:2: an action name in keys must be plain text"},
		{"nested mapping", "keys:\n  down: {a: b}\n", `config.yml:2: a key of "down" must be a non-empty text value`},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			_, err := Parse("config.yml", []byte(testCase.yaml))
			if err == nil || !strings.HasPrefix(err.Error(), testCase.want) {
				test.Errorf("error = %v, want it to start with %q", err, testCase.want)
			}
		})
	}
}
