// Package gitrepo infers which GitHub repository a working directory belongs
// to by reading its git remotes. It has no UI dependencies; git is run through
// an injectable Runner so tests never need a real checkout.
package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
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
	Repo github.Repo
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
func Infer(ctx context.Context, run Runner, dir string) (Remote, error) {
	if _, err := run(ctx, dir, "rev-parse", "--show-toplevel"); err != nil {
		return Remote{}, fmt.Errorf("%w: %w", ErrNotCheckout, err)
	}
	for _, name := range remoteNames {
		raw, err := run(ctx, dir, "remote", "get-url", name)
		if err != nil {
			continue // remote not configured
		}
		if r, err := ParseURL(raw); err == nil {
			return r, nil
		}
	}
	return Remote{}, ErrNoGitHubRemote
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
	repo, err := github.ParseRepo(path)
	if err != nil || host == "" {
		return Remote{}, fmt.Errorf("parse remote %q: want host and owner/repo", raw)
	}
	return Remote{Host: host, Repo: repo}, nil
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
	case "ssh", "git":
		host = u.Hostname() // the port is the SSH port, not the web host's
	case "https", "http":
		host = u.Host
	default:
		return "", "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	// GitHub serves SSH over 443 from this alias.
	if host == "ssh.github.com" {
		host = "github.com"
	}
	return host, u.Path, nil
}
