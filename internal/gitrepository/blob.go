package gitrepository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
// diff's index line. It fails with ErrBlobNotFound when the checkout has no
// such blob, or when the abbreviation matches more than one blob.
//
// The identifier is only ever matched against objects: a branch or tag that
// happens to be named like it is ignored (git itself prefers the ref), and an
// abbreviation shared with a commit or tree still finds the one blob. Git is
// never allowed to fetch a missing object (a partial clone would otherwise go
// to the network) or to prompt.
//
// Unlike Runner it does not trim the output: a file's trailing newline, or its
// absence, is content.
func ReadBlob(ctx context.Context, directory, blobObjectIdentifier string) ([]byte, error) {
	// Only a hexadecimal name is accepted so the identifier can never be read
	// by git as an option or as a revision expression such as HEAD:path.
	if !isObjectName(blobObjectIdentifier) {
		return nil, fmt.Errorf("%w: %q is not a hexadecimal object name", ErrBlobNotFound, blobObjectIdentifier)
	}
	fullIdentifier, err := resolveBlob(ctx, directory, blobObjectIdentifier)
	if err != nil {
		return nil, err
	}
	// A full-length hexadecimal name is always read as an object, never a ref.
	contents, err := runLocalGit(ctx, directory, nil, "cat-file", "blob", fullIdentifier)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", blobObjectIdentifier, err)
	}
	return contents, nil
}

// resolveBlob returns the full name of the one blob whose name starts with
// abbreviation. rev-parse --disambiguate lists objects only, never refs.
func resolveBlob(ctx context.Context, directory, abbreviation string) (string, error) {
	candidates, err := runLocalGit(ctx, directory, nil, "rev-parse", "--disambiguate="+abbreviation)
	if err != nil {
		return "", fmt.Errorf("resolve blob %s: %w", abbreviation, err)
	}
	if len(bytes.TrimSpace(candidates)) == 0 {
		return "", fmt.Errorf("%w: %s", ErrBlobNotFound, abbreviation)
	}
	types, err := runLocalGit(ctx, directory, bytes.NewReader(candidates), "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return "", fmt.Errorf("resolve blob %s: %w", abbreviation, err)
	}
	var blobs []string
	for line := range strings.Lines(string(types)) {
		if name, objectType, ok := strings.Cut(strings.TrimSpace(line), " "); ok && objectType == "blob" {
			blobs = append(blobs, name)
		}
	}
	if len(blobs) != 1 {
		return "", fmt.Errorf("%w: %s matches %d blobs", ErrBlobNotFound, abbreviation, len(blobs))
	}
	return blobs[0], nil
}

// runLocalGit runs git in directory with input on standard input and returns
// its untrimmed standard output. Git may neither fetch missing objects nor
// prompt. A failure carries git's diagnostics; a cancelled ctx yields its error.
func runLocalGit(ctx context.Context, directory string, input io.Reader, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	command.Env = append(environment.Scrub(os.Environ()), "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0")
	command.WaitDelay = subprocessWaitDelay
	command.Stdin = input
	var output, diagnostics bytes.Buffer
	command.Stdout = &output
	command.Stderr = &diagnostics
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("git %s: %w: %s", arguments[0], err, strings.TrimSpace(diagnostics.String()))
	}
	return output.Bytes(), nil
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
	// environment has no GIT_SSH_COMMAND of its own. An unset key is not an
	// error.
	configuredSSHCommand := ""
	if !hasVariable(os.Environ(), "GIT_SSH_COMMAND") {
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
		// --refmap= stops the configured fetch refspecs (such as a
		// refs/pull/*/head mapping) writing remote-tracking refs; the
		// fetch must not start maintenance or touch submodules either.
		"fetch", "--no-write-fetch-head", "--refmap=", "--no-tags",
		"--no-auto-maintenance", "--no-recurse-submodules", "--quiet", "--",
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
//
// Git picks the ssh command from GIT_SSH_COMMAND, then core.sshCommand, then
// GIT_SSH. A user's GIT_SSH_COMMAND is respected as it is. A configured
// core.sshCommand is kept with BatchMode added, since our GIT_SSH_COMMAND would
// otherwise replace it. A GIT_SSH program (which may not take -o, as plink
// doesn't) is left alone when nothing above it is set.
func fetchEnvironment(variables []string, configuredSSHCommand string) []string {
	variables = append(variables, "GIT_TERMINAL_PROMPT=0")
	if hasVariable(variables, "GIT_SSH_COMMAND") {
		return variables
	}
	if configuredSSHCommand != "" {
		return append(variables, "GIT_SSH_COMMAND="+configuredSSHCommand+" -o BatchMode=yes")
	}
	if hasVariable(variables, "GIT_SSH") {
		return variables
	}
	return append(variables, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
}

// hasVariable reports whether an exec environment sets name.
func hasVariable(variables []string, name string) bool {
	for _, variable := range variables {
		if variableName, _, _ := strings.Cut(variable, "="); variableName == name {
			return true
		}
	}
	return false
}
