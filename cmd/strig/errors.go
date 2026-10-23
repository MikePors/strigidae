package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/MikePors/strigidae/internal/gitrepository"
)

// usageError is a mistake on the command line; it exits 2. An empty msg means
// the flag package has already reported it.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// signInError means there are no usable credentials for host ("" for the gh
// default host).
type signInError struct {
	host string
	err  error
}

func (e *signInError) Error() string { return e.err.Error() }
func (e *signInError) Unwrap() error { return e.err }

// report writes err to stderr in the form the user should see and returns the
// process exit code. It is the only place run turns errors into output.
func report(stderr io.Writer, err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	msg, code := message(err)
	if msg != "" {
		_, _ = fmt.Fprintf(stderr, "strig: %s\n", msg) // best effort: nowhere else to report
	}
	return code
}

func message(err error) (string, int) {
	var (
		usage   *usageError
		signIn  *signInError
		unknown *gitrepository.UnknownHostError
	)
	switch {
	case errors.As(err, &usage):
		return usage.msg, 2
	case errors.As(err, &signIn):
		login := "gh auth login"
		if signIn.host != "" {
			login += " --hostname " + signIn.host
		}
		return fmt.Sprintf("not signed in to %s: run `%s` and try again", cmp.Or(signIn.host, "GitHub"), login), 1
	case errors.As(err, &unknown):
		return fmt.Sprintf("remote host %s isn't a GitHub host gh is signed in to; pass -R owner/repo or run gh auth login --hostname <host>", unknown.QuotedHosts()), 1
	case errors.Is(err, gitrepository.ErrNotCheckout):
		return "run strig inside a GitHub repository checkout, or pass -R owner/repo", 1
	case errors.Is(err, gitrepository.ErrNoGitHubRemote):
		return "no GitHub remote (upstream or origin) in this checkout; pass -R owner/repo", 1
	default:
		return err.Error(), 1
	}
}
