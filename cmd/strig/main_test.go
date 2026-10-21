package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	cliBrowser "github.com/cli/browser"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

// harness records what run did with its injected dependencies.
type harness struct {
	deps       deps
	clientHost *string
	started    *github.Repo
}

// newHarness returns deps whose checkout has the given remotes ("" key
// "toplevel" absent means not a checkout).
func newHarness(remotes map[string]string, checkout bool, clientErr error) harness {
	h := harness{clientHost: new(string), started: new(github.Repo)}
	h.deps = deps{
		getwd: func() (string, error) { return "/work", nil },
		runner: func(_ context.Context, _ string, args ...string) (string, error) {
			if args[0] == "rev-parse" {
				if !checkout {
					return "", errors.New("not a git repository")
				}
				return "/work", nil
			}
			if u, ok := remotes[args[2]]; ok {
				return u, nil
			}
			return "", errors.New("no such remote")
		},
		knownHost: func(host string) bool { return host == "github.com" || host == "ghe.example.com" },
		newClient: func(host string) (github.Client, error) {
			*h.clientHost = host
			if clientErr != nil {
				return nil, fmt.Errorf("host %s: %w", host, clientErr)
			}
			return fake.New(), nil
		},
		start: func(a app, _ io.Writer) error {
			*h.started = a.repo
			return nil
		},
	}
	return h
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHarness(nil, false, nil)
	if code := run([]string{"--version"}, &stdout, &stderr, h.deps); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "strig ") {
		t.Fatalf("stdout = %q, want prefix %q", stdout.String(), "strig ")
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--nope"}, &stdout, &stderr, newHarness(nil, false, nil).deps); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunPositionalArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHarness(map[string]string{"origin": "git@github.com:o/r.git"}, true, nil)
	if code := run([]string{"cli/cli"}, &stdout, &stderr, h.deps); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if want := `strig: unexpected argument "cli/cli" (did you mean -R cli/cli?)`; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if !h.started.IsZero() {
		t.Error("TUI started despite the stray argument")
	}
}

func TestSilenceBrowser(t *testing.T) {
	oldOut, oldErr := cliBrowser.Stdout, cliBrowser.Stderr
	t.Cleanup(func() { cliBrowser.Stdout, cliBrowser.Stderr = oldOut, oldErr })
	var out, errOut bytes.Buffer
	cliBrowser.Stdout, cliBrowser.Stderr = &out, &errOut

	silenceBrowser()
	if cliBrowser.Stdout != io.Discard || cliBrowser.Stderr != io.Discard {
		t.Fatal("browser output not discarded")
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHarness(nil, false, nil)
	for _, flagName := range []string{"-h", "--help"} {
		if code := run([]string{flagName}, &stdout, &stderr, h.deps); code != 0 {
			t.Errorf("%s exit code = %d, want 0", flagName, code)
		}
	}
	for _, want := range []string{"-log-path", "-R owner/repo"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("usage should mention %q: %q", want, stderr.String())
		}
	}
}

func TestRunLogPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--log-path"}, &stdout, &stderr, newHarness(nil, false, nil).deps); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if got, want := stdout.String(), "/xdg/state/strig/strig.log\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunRepoSelection(t *testing.T) {
	origin := "git@github.com:fork/widgets.git"
	upstream := "https://ghe.example.com/acme/widgets.git"
	tests := []struct {
		name       string
		args       []string
		remotes    map[string]string
		checkout   bool
		clientErr  error
		wantCode   int
		wantRepo   string
		wantHost   string
		wantClient bool // newClient should have been called
		wantStderr string
	}{
		{
			name: "inferred from origin", remotes: map[string]string{"origin": origin}, checkout: true,
			wantRepo: "fork/widgets", wantHost: "github.com", wantClient: true,
		},
		{
			name: "upstream wins and sets host", remotes: map[string]string{"origin": origin, "upstream": upstream}, checkout: true,
			wantRepo: "acme/widgets", wantHost: "ghe.example.com", wantClient: true,
		},
		{
			name: "gitlab upstream falls back to github origin", remotes: map[string]string{"origin": origin, "upstream": "git@gitlab.com:acme/widgets.git"}, checkout: true,
			wantRepo: "fork/widgets", wantHost: "github.com", wantClient: true,
		},
		{
			name: "ssh alias host is named in the error", remotes: map[string]string{"origin": "github-work:acme/app.git"}, checkout: true,
			wantCode: 1, wantStderr: `remote host "github-work" isn't a GitHub host gh is signed in to; pass -R owner/repo or run gh auth login --hostname <host>`,
		},
		{
			name: "two unknown hosts are quoted individually", remotes: map[string]string{"upstream": "git@gitlab.com:o/r.git", "origin": "github-work:acme/app.git"}, checkout: true,
			wantCode: 1, wantStderr: `remote host "gitlab.com", "github-work" isn't a GitHub host`,
		},
		{
			name: "-R overrides inference", args: []string{"-R", "o/r"}, remotes: map[string]string{"origin": origin}, checkout: true,
			wantRepo: "o/r", wantHost: "", wantClient: true,
		},
		{
			name: "-R works outside a checkout", args: []string{"-R", "o/r"},
			wantRepo: "o/r", wantClient: true,
		},
		{
			name: "-R invalid", args: []string{"-R", "nonsense"}, checkout: true,
			wantCode: 2, wantStderr: "invalid repository",
		},
		{
			name:     "outside a checkout",
			wantCode: 1, wantStderr: "strig: run strig inside a GitHub repository checkout, or pass -R owner/repo\n",
		},
		{
			name: "checkout without remotes", checkout: true,
			wantCode: 1, wantStderr: "no GitHub remote",
		},
		{
			name: "not authenticated", remotes: map[string]string{"origin": origin}, checkout: true,
			clientErr: github.ErrNotAuthenticated,
			wantCode:  1, wantHost: "github.com", wantClient: true, wantStderr: "not signed in to github.com: run `gh auth login --hostname github.com`",
		},
		{
			name: "client failure", remotes: map[string]string{"origin": origin}, checkout: true,
			clientErr: errors.New("boom"),
			wantCode:  1, wantHost: "github.com", wantClient: true, wantStderr: "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(tt.remotes, tt.checkout, tt.clientErr)
			var clientCalled bool
			inner := h.deps.newClient
			h.deps.newClient = func(host string) (github.Client, error) { clientCalled = true; return inner(host) }

			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr, h.deps)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
			if clientCalled != tt.wantClient {
				t.Errorf("client constructed = %v, want %v", clientCalled, tt.wantClient)
			}
			if tt.wantClient && *h.clientHost != tt.wantHost {
				t.Errorf("client host = %q, want %q", *h.clientHost, tt.wantHost)
			}
			if got := h.started.String(); tt.wantRepo != "" && got != tt.wantRepo {
				t.Errorf("started on %q, want %q", got, tt.wantRepo)
			}
			if tt.wantRepo == "" && !h.started.IsZero() {
				t.Errorf("TUI started on %v despite failure", *h.started)
			}
		})
	}
}
