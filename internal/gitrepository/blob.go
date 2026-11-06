package gitrepository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/MikePors/strigidae/internal/environment"
)

// ErrBlobNotFound means the checkout has no blob with the requested object
// identifier, or the abbreviation matches more than one object.
var ErrBlobNotFound = errors.New("blob not found in the checkout")

// subprocessWaitDelay bounds how long a cancelled git is waited for. Without
// it, an ssh child that outlives git would keep the output pipe open and block
// the caller past its deadline.
const subprocessWaitDelay = 2 * time.Second

// ReadBlob returns the exact contents of the blob blobObjectIdentifier in the
// checkout containing directory. The identifier may be abbreviated, as in a
// diff's index line. It fails with ErrBlobNotFound when the blob is missing or
// the abbreviation is ambiguous.
//
// Unlike Runner it does not trim the output: a file's trailing newline, or its
// absence, is content.
func ReadBlob(ctx context.Context, directory, blobObjectIdentifier string) ([]byte, error) {
	// Only a hexadecimal name is accepted so the identifier can never be read
	// by git as an option or as a revision expression such as HEAD:path.
	if !isObjectName(blobObjectIdentifier) {
		return nil, fmt.Errorf("%w: %q is not a hexadecimal object name", ErrBlobNotFound, blobObjectIdentifier)
	}
	command := exec.CommandContext(ctx, "git", "cat-file", "blob", blobObjectIdentifier)
	command.Dir = directory
	command.Env = environment.Scrub(os.Environ())
	command.WaitDelay = subprocessWaitDelay
	var contents, diagnostics bytes.Buffer
	command.Stdout = &contents
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("read blob %s: %w", blobObjectIdentifier, ctx.Err())
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return nil, fmt.Errorf("%w: %s: %s", ErrBlobNotFound, blobObjectIdentifier, strings.TrimSpace(diagnostics.String()))
		}
		return nil, fmt.Errorf("read blob %s: %w", blobObjectIdentifier, err)
	}
	return contents.Bytes(), nil
}

// isObjectName reports whether text could be a full or abbreviated object name:
// at least four hexadecimal digits (git's minimum abbreviation) and at most 64
// (a SHA-256 name).
func isObjectName(text string) bool {
	if len(text) < 4 || len(text) > 64 {
		return false
	}
	for _, character := range text {
		isDigit := character >= '0' && character <= '9'
		isLowerHexLetter := character >= 'a' && character <= 'f'
		isUpperHexLetter := character >= 'A' && character <= 'F'
		if !isDigit && !isLowerHexLetter && !isUpperHexLetter {
			return false
		}
	}
	return true
}

// FetchPullRequestHead downloads the objects of pull request number's head from
// the git remote remoteName of the checkout containing directory, so that the
// pull request's blobs can be read with ReadBlob.
//
// It writes no ref and no FETCH_HEAD, so the checkout looks untouched to the
// user afterwards (git status, branches, and the next `git fetch` all behave as
// before); the objects stay in the object database until git prunes them. It
// never prompts: standard input is closed, terminal and SSH prompts are
// disabled, and cancelling ctx stops it.
func FetchPullRequestHead(ctx context.Context, directory, remoteName string, number int) error {
	if number <= 0 {
		return fmt.Errorf("fetch pull request head: invalid number %d", number)
	}
	if remoteName == "" || strings.HasPrefix(remoteName, "-") {
		return fmt.Errorf("fetch pull request head: invalid remote name %q", remoteName)
	}
	// The configured ssh command (core.sshCommand) is only consulted when the
	// environment has no override of its own. An unset key is not an error.
	configuredSSHCommand := ""
	if !overridesSSHCommand(os.Environ()) {
		configuredSSHCommand, _ = ExecRunner(ctx, directory, "config", "--get", "core.sshCommand")
	}
	command := newFetchCommand(ctx, directory, remoteName, number, os.Environ(), configuredSSHCommand)
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("fetch pull request %d head from %s: %w", number, remoteName, ctx.Err())
		}
		return fmt.Errorf("fetch pull request %d head from %s: %w: %s", number, remoteName, err, strings.TrimSpace(diagnostics.String()))
	}
	return nil
}

// newFetchCommand builds the git fetch for FetchPullRequestHead. It is separate
// so tests can inspect the arguments and environment without running git.
func newFetchCommand(
	ctx context.Context,
	directory, remoteName string,
	number int,
	baseEnvironment []string,
	configuredSSHCommand string,
) *exec.Cmd {
	command := exec.CommandContext(ctx, "git",
		"fetch", "--no-write-fetch-head", "--no-tags", "--quiet", "--",
		remoteName, "refs/pull/"+strconv.Itoa(number)+"/head",
	)
	command.Dir = directory
	command.Stdin = nil // the null device: there is nothing to answer a prompt with
	command.Env = fetchEnvironment(environment.Scrub(baseEnvironment), configuredSSHCommand)
	command.WaitDelay = subprocessWaitDelay
	return command
}

// fetchEnvironment adds the variables that keep git from ever prompting. Later
// entries win over earlier ones in an exec environment.
func fetchEnvironment(variables []string, configuredSSHCommand string) []string {
	variables = append(variables, "GIT_TERMINAL_PROMPT=0")
	if overridesSSHCommand(variables) {
		return variables // the user chose how ssh is run; respect it
	}
	// GIT_SSH_COMMAND beats core.sshCommand, so keep the configured command and
	// only add the option that stops ssh asking for a passphrase or host key.
	sshCommand := "ssh"
	if configuredSSHCommand != "" {
		sshCommand = configuredSSHCommand
	}
	return append(variables, "GIT_SSH_COMMAND="+sshCommand+" -o BatchMode=yes")
}

// overridesSSHCommand reports whether the user set GIT_SSH_COMMAND, or the older
// GIT_SSH, which our own GIT_SSH_COMMAND would otherwise silently replace.
func overridesSSHCommand(variables []string) bool {
	for _, variable := range variables {
		name, _, _ := strings.Cut(variable, "=")
		if name == "GIT_SSH_COMMAND" || name == "GIT_SSH" {
			return true
		}
	}
	return false
}
