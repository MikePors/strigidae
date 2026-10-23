package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/gitrepository"
)

func TestReport(test *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   int
		wantStderr string
	}{
		{name: "success", err: nil},
		{name: "help", err: flag.ErrHelp},
		{name: "usage already printed", err: &usageError{}, wantCode: 2},
		{name: "usage", err: &usageError{"-R: bad"}, wantCode: 2, wantStderr: "strig: -R: bad\n"},
		{
			name: "sign in to default host", err: &signInError{err: github.ErrNotAuthenticated}, wantCode: 1,
			wantStderr: "strig: not signed in to GitHub: run `gh auth login` and try again\n",
		},
		{
			name: "sign in to named host", err: fmt.Errorf("wrapped: %w", &signInError{host: "ghe.example.com", err: github.ErrNotAuthenticated}), wantCode: 1,
			wantStderr: "strig: not signed in to ghe.example.com: run `gh auth login --hostname ghe.example.com` and try again\n",
		},
		{
			name: "unknown host", err: &gitrepository.UnknownHostError{Hosts: []string{"a", "b"}}, wantCode: 1,
			wantStderr: `strig: remote host "a", "b" isn't a GitHub host gh is signed in to; pass -R owner/repo or run gh auth login --hostname <host>` + "\n",
		},
		{name: "anything else", err: errors.New("boom"), wantCode: 1, wantStderr: "strig: boom\n"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var stderr bytes.Buffer
			if code := report(&stderr, testCase.err); code != testCase.wantCode {
				test.Errorf("exit code = %d, want %d", code, testCase.wantCode)
			}
			if got := stderr.String(); got != testCase.wantStderr {
				test.Errorf("stderr = %q, want %q", got, testCase.wantStderr)
			}
		})
	}
}

func TestRunStartFailure(test *testing.T) {
	harness := newHarness(nil, false, nil)
	harness.dependencies.start = func(application, io.Writer) error { return errors.New("run: no tty") }
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-R", "o/r"}, &stdout, &stderr, harness.dependencies); code != 1 {
		test.Fatalf("exit code = %d, want 1", code)
	}
	if got, want := stderr.String(), "strig: run: no tty\n"; got != want {
		test.Errorf("stderr = %q, want %q", got, want)
	}
}
