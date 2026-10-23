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

func TestReport(t *testing.T) {
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
		{
			name: "not a checkout", err: fmt.Errorf("%w: exit 128", gitrepository.ErrNotCheckout), wantCode: 1,
			wantStderr: "strig: run strig inside a GitHub repository checkout, or pass -R owner/repo\n",
		},
		{
			name: "no github remote", err: gitrepository.ErrNoGitHubRemote, wantCode: 1,
			wantStderr: "strig: no GitHub remote (upstream or origin) in this checkout; pass -R owner/repo\n",
		},
		{name: "anything else", err: errors.New("boom"), wantCode: 1, wantStderr: "strig: boom\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := report(&stderr, tt.err); code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestRunStartFailure(t *testing.T) {
	h := newHarness(nil, false, nil)
	h.deps.start = func(app, io.Writer) error { return errors.New("run: no tty") }
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-R", "o/r"}, &stdout, &stderr, h.deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if got, want := stderr.String(), "strig: run: no tty\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}
