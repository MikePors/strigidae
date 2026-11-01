package config

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParsePlugins(test *testing.T) {
	for _, testCase := range []struct {
		name string
		yaml string
		want []Plugin
	}{
		{
			"all keys",
			"plugins:\n  - command: ask\n    on_run: [strig-ask, --fast, '']\n    target: lines\n    timeout: 2m\n",
			[]Plugin{{Command: "ask", OnRun: []string{"strig-ask", "--fast", ""}, Target: TargetLines, Timeout: 2 * time.Minute}},
		},
		{
			"timeout defaults to 60s",
			"plugins:\n  - {command: summarize, on_run: [summarizer], target: pull-request}\n",
			[]Plugin{{Command: "summarize", OnRun: []string{"summarizer"}, Target: TargetPullRequest, Timeout: 60 * time.Second}},
		},
		{
			"two plugins keep file order",
			"plugins:\n  - {command: b-2, on_run: [x], target: lines}\n  - {command: A1, on_run: [y, '1'], target: lines}\n",
			[]Plugin{
				{Command: "b-2", OnRun: []string{"x"}, Target: TargetLines, Timeout: DefaultPluginTimeout},
				{Command: "A1", OnRun: []string{"y", "1"}, Target: TargetLines, Timeout: DefaultPluginTimeout},
			},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := Parse("c.yml", []byte(testCase.yaml))
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got.Plugins); diff != "" {
				test.Errorf("plugins mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParsePluginsAbsent(test *testing.T) {
	got, err := Parse("c.yml", []byte("theme: x\n"))
	if err != nil {
		test.Fatal(err)
	}
	if got.Plugins != nil {
		test.Errorf("Plugins = %v, want none", got.Plugins)
	}
}

func TestParsePluginErrors(test *testing.T) {
	for _, testCase := range []struct {
		name string
		yaml string
		want string
	}{
		{"empty list", "plugins: []\n", "c.yml:1: plugins must be a non-empty list"},
		{"not a list", "plugins: ask\n", "c.yml:1: plugins must be a non-empty list"},
		{"not a mapping", "plugins:\n  - ask\n", "c.yml:2: a plugin must be a mapping of keys to values"},
		{"missing command", "plugins:\n  - {on_run: [x], target: lines}\n", "c.yml:2: plugin is missing a command"},
		{"missing on_run", "plugins:\n  - {command: ask, target: lines}\n", `c.yml:2: plugin "ask" is missing on_run`},
		{"missing target", "plugins:\n  - {command: ask, on_run: [x]}\n", `c.yml:2: plugin "ask" is missing a target`},
		{"command with a space", "plugins:\n  - {command: 'ask me', on_run: [x], target: lines}\n", `c.yml:2: plugin command "ask me" must be a plain word (letters, digits and hyphens)`},
		{"command with a colon", "plugins:\n  - {command: ':ask', on_run: [x], target: lines}\n", `c.yml:2: plugin command ":ask" must be a plain word (letters, digits and hyphens)`},
		{"command with an underscore", "plugins:\n  - {command: ask_me, on_run: [x], target: lines}\n", `c.yml:2: plugin command "ask_me" must be a plain word (letters, digits and hyphens)`},
		{"empty command", "plugins:\n  - {command: '', on_run: [x], target: lines}\n", "c.yml:2: plugin command must be a non-empty text value"},
		{"duplicate command", "plugins:\n  - {command: ask, on_run: [x], target: lines}\n  - {command: ask, on_run: [y], target: lines}\n", `c.yml:3: duplicate plugin command "ask"`},
		{"on_run is text", "plugins:\n  - {command: ask, on_run: x, target: lines}\n", "c.yml:2: on_run must be a non-empty list"},
		{"on_run is empty", "plugins:\n  - {command: ask, on_run: [], target: lines}\n", "c.yml:2: on_run must be a non-empty list"},
		{"on_run program is empty", "plugins:\n  - {command: ask, on_run: [''], target: lines}\n", "c.yml:2: on_run program must be a non-empty text value"},
		{"on_run program is blank", "plugins:\n  - {command: ask, on_run: ['  ', x], target: lines}\n", "c.yml:2: on_run program must be a non-empty text value"},
		{"on_run argument is a list", "plugins:\n  - {command: ask, on_run: [x, [y]], target: lines}\n", "c.yml:2: every item of on_run must be text"},
		{"on_run argument is null", "plugins:\n  - {command: ask, on_run: [x, ~], target: lines}\n", "c.yml:2: every item of on_run must be text"},
		{"bad target", "plugins:\n  - {command: ask, on_run: [x], target: file}\n", `c.yml:2: target "file" is not one of: lines, pull-request`},
		{"zero timeout", "plugins:\n  - {command: ask, on_run: [x], target: lines, timeout: 0s}\n", "c.yml:2: plugin timeout must be a positive duration such as 10s or 1m30s"},
		{"timeout without unit", "plugins:\n  - {command: ask, on_run: [x], target: lines, timeout: 30}\n", "c.yml:2: plugin timeout must be a positive duration such as 10s or 1m30s"},
		{"negative timeout", "plugins:\n  - {command: ask, on_run: [x], target: lines, timeout: -1m}\n", "c.yml:2: plugin timeout must be a positive duration such as 10s or 1m30s"},
		{"unknown key", "plugins:\n  - {command: ask, on_run: [x], target: lines, show: inline}\n", `c.yml:2: unknown key "show" in a plugin (valid keys: command, on_run, target, timeout)`},
		{"duplicate key", "plugins:\n  - {command: ask, command: b, on_run: [x], target: lines}\n", `c.yml:2: duplicate key "command" in a plugin`},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			_, err := Parse("c.yml", []byte(testCase.yaml))
			var configError *Error
			if !errors.As(err, &configError) {
				test.Fatalf("err = %v, want *Error", err)
			}
			if diff := cmp.Diff(testCase.want, err.Error()); diff != "" {
				test.Errorf("message mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
