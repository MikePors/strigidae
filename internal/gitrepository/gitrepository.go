// Package gitrepo infers which GitHub repository a working directory belongs
// to by reading its git remotes. It has no UI dependencies; git is run through
// an injectable Runner so tests never need a real checkout.
package gitrepository

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

var (
	// ErrNotCheckout means the directory is not inside a git work tree (or git
	// is not installed).
	ErrNotCheckout = errors.New("not inside a git checkout")
	// ErrNoGitHubRemote means the checkout has no usable "upstream" or
	// "origin" remote.
	ErrNoGitHubRemote = errors.New("no GitHub remote named upstream or origin")
)

// remoteNames are the remotes to try, in order. "upstream" wins so that a fork
// checkout lists the parent repository's pull requests.
var remoteNames = []string{"upstream", "origin"}

// Remote is a repository on a specific host.
type Remote struct {
	// Host is the GitHub host, e.g. "github.com" or a GitHub Enterprise host.
	Host string
	Repo github.Repository
}

// Runner runs git with args in dir and returns its trimmed standard output.
type Runner func(ctx context.Context, dir string, args ...string) (string, error)

// ExecRunner is the Runner that executes the real git binary.
func ExecRunner(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Infer finds the GitHub repository for the checkout containing dir.
//
// A remote whose host knownHost rejects is skipped, so a mirror on another
// forge falls through to the next remote. A nil knownHost accepts every host.
func Infer(ctx context.Context, run Runner, dir string, knownHost func(host string) bool) (Remote, error) {
	if _, err := run(ctx, dir, "rev-parse", "--show-toplevel"); err != nil {
		return Remote{}, fmt.Errorf("%w: %w", ErrNotCheckout, err)
	}
	var unknown []string
	for _, name := range remoteNames {
		raw, err := run(ctx, dir, "remote", "get-url", name)
		if err != nil {
			continue // remote not configured
		}
		r, err := ParseURL(raw)
		if err != nil {
			continue
		}
		if knownHost != nil && !knownHost(r.Host) {
			unknown = append(unknown, r.Host)
			continue
		}
		return r, nil
	}
	if len(unknown) > 0 {
		return Remote{}, &UnknownHostError{Hosts: unknown}
	}
	return Remote{}, ErrNoGitHubRemote
}

// UnknownHostError means the checkout has parseable remotes, but none on a
// host the caller recognises (for example an SSH alias or another forge).
type UnknownHostError struct {
	// Hosts are the hosts of the rejected remotes, in preference order.
	Hosts []string
}

// Error implements error.
func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("remote host %s is not a known GitHub host", e.QuotedHosts())
}

// QuotedHosts returns the hosts individually quoted and comma-separated.
func (e *UnknownHostError) QuotedHosts() string {
	q := make([]string, len(e.Hosts))
	for i, h := range e.Hosts {
		q[i] = strconv.Quote(h)
	}
	return strings.Join(q, ", ")
}

// ParseURL parses a git remote URL into a host and repository. It accepts
// scp-like SSH (git@host:owner/repo.git), ssh://, git://, https:// and http://.
func ParseURL(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	host, path, err := splitURL(raw)
	if err != nil {
		return Remote{}, fmt.Errorf("parse remote %q: %w", raw, err)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	repo, err := github.ParseRepository(path)
	if err != nil || host == "" {
		return Remote{}, fmt.Errorf("parse remote %q: want host and owner/repo", raw)
	}
	return Remote{Host: github.NormalizeHost(host), Repo: repo}, nil
}

func splitURL(raw string) (host, path string, err error) {
	if !strings.Contains(raw, "://") {
		// scp-like: [user@]host:path
		hostPart, p, ok := strings.Cut(raw, ":")
		if !ok || strings.Contains(hostPart, "/") {
			return "", "", errors.New("unrecognised URL")
		}
		if _, h, found := strings.Cut(hostPart, "@"); found {
			hostPart = h
		}
		return hostPart, p, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	switch u.Scheme {
	case "ssh", "git", "https", "http":
		host = u.Hostname() // any port is a transport detail, not part of the host
	default:
		return "", "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	return host, u.Path, nil
}
