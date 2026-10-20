package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "strig ") {
		t.Fatalf("stdout = %q, want prefix %q", stdout.String(), "strig ")
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, flagName := range []string{"-h", "--help"} {
		if code := run([]string{flagName}, &stdout, &stderr); code != 0 {
			t.Errorf("%s exit code = %d, want 0", flagName, code)
		}
	}
	if !strings.Contains(stderr.String(), "-log-path") {
		t.Errorf("usage should mention -log-path: %q", stderr.String())
	}
}

func TestRunLogPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--log-path"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if got, want := stdout.String(), "/xdg/state/strig/strig.log\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
