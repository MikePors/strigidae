package gitrepository

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// runGit runs the real git in directory with an identity and no user or system
// configuration, so the tests behave the same on every machine and in CI.
func runGit(test *testing.T, directory string, arguments ...string) string {
	test.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		test.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// newRepository makes an empty repository with one commit in a temporary
// directory, skipping the test when git is not installed.
func newRepository(test *testing.T) string {
	test.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		test.Skip("git is not installed")
	}
	// Tests run git with the process environment, so keep the user's and the
	// system's configuration out of it.
	test.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	test.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	directory := test.TempDir()
	runGit(test, directory, "init", "--quiet", "--initial-branch=main")
	runGit(test, directory, "commit", "--quiet", "--allow-empty", "--message=first")
	return directory
}

func writeBlob(test *testing.T, directory string, contents []byte) string {
	test.Helper()
	path := filepath.Join(directory, "file.txt")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		test.Fatal(err)
	}
	return runGit(test, directory, "hash-object", "-w", "--no-filters", "file.txt")
}

func TestReadBlobIsExact(test *testing.T) {
	directory := newRepository(test)
	for _, testCase := range []struct {
		name     string
		contents string
	}{
		{"trailing newline", "one\ntwo\n"},
		{"no final newline", "one\ntwo"},
		{"several trailing newlines", "one\n\n\n"},
		{"leading and trailing whitespace", "  \tone \n "},
		{"carriage returns", "one\r\ntwo\r\n"},
		{"empty", ""},
		{"binary", "a\x00b\xffc\n"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			identifier := writeBlob(test, directory, []byte(testCase.contents))
			got, err := ReadBlob(context.Background(), directory, identifier)
			if err != nil {
				test.Fatal(err)
			}
			if string(got) != testCase.contents {
				test.Errorf("contents = %q, want %q", got, testCase.contents)
			}
			abbreviated, err := ReadBlob(context.Background(), directory, identifier[:8])
			if err != nil {
				test.Fatalf("abbreviated: %v", err)
			}
			if string(abbreviated) != testCase.contents {
				test.Errorf("abbreviated contents = %q, want %q", abbreviated, testCase.contents)
			}
		})
	}
}

func TestReadBlobNotFound(test *testing.T) {
	directory := newRepository(test)
	existing := writeBlob(test, directory, []byte("present\n"))
	commit := runGit(test, directory, "rev-parse", "HEAD")
	for _, testCase := range []struct {
		name       string
		identifier string
	}{
		{"missing", strings.Repeat("a", 40)},
		{"missing abbreviation", "deadbeef"},
		{"a commit is not a blob", commit},
		{"not hexadecimal", "HEAD:file.txt"},
		{"looks like an option", "--batch"},
		{"too short", existing[:3]},
		{"empty", ""},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := ReadBlob(context.Background(), directory, testCase.identifier)
			if !errors.Is(err, ErrBlobNotFound) {
				test.Fatalf("err = %v, want ErrBlobNotFound", err)
			}
			if got != nil {
				test.Errorf("contents = %q, want none", got)
			}
		})
	}
}

func TestReadBlobDistinguishesCancellation(test *testing.T) {
	directory := newRepository(test)
	identifier := writeBlob(test, directory, []byte("x"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadBlob(ctx, directory, identifier)
	if errors.Is(err, ErrBlobNotFound) || !errors.Is(err, context.Canceled) {
		test.Errorf("err = %v, want context.Canceled and not ErrBlobNotFound", err)
	}
}

// pullRequestFixture is a checkout whose remote "origin" is a second local
// repository holding refs/pull/1/head, standing in for GitHub.
type pullRequestFixture struct {
	checkout   string
	remote     string
	identifier string // the blob that only the pull request has
}

func newPullRequestFixture(test *testing.T) pullRequestFixture {
	test.Helper()
	remote := newRepository(test)
	identifier := writeBlob(test, remote, []byte("only in the pull request\n"))
	runGit(test, remote, "add", "file.txt")
	tree := runGit(test, remote, "write-tree")
	commit := runGit(test, remote, "commit-tree", tree, "-p", "HEAD", "-m", "pull request")
	runGit(test, remote, "update-ref", "refs/pull/1/head", commit)
	// A clone only fetches branches, so it does not have the pull request's
	// commit or blob; --no-local stops it copying the whole object directory.
	checkout := filepath.Join(test.TempDir(), "checkout")
	runGit(test, filepath.Dir(checkout), "clone", "--quiet", "--no-local", remote, checkout)
	return pullRequestFixture{checkout: checkout, remote: remote, identifier: identifier}
}

// snapshot lists everything FetchPullRequestHead must leave alone.
func snapshot(test *testing.T, directory string) (references string, fetchHeadExists bool) {
	test.Helper()
	references = runGit(test, directory, "for-each-ref", "--format=%(refname) %(objectname)")
	references += "\n" + runGit(test, directory, "status", "--porcelain=v2", "--branch")
	_, err := os.Stat(filepath.Join(directory, ".git", "FETCH_HEAD"))
	return references, err == nil
}

func TestFetchPullRequestHeadMakesTheBlobReadable(test *testing.T) {
	for _, testCase := range []struct {
		name   string
		config []string // git config key and value pairs for the checkout
	}{
		{"default configuration", nil},
		{"pull requests mapped to remote-tracking refs", []string{"--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/pr/*"}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			fixture := newPullRequestFixture(test)
			if testCase.config != nil {
				runGit(test, fixture.checkout, append([]string{"config"}, testCase.config...)...)
			}
			assertFetchMakesTheBlobReadable(test, fixture)
		})
	}
}

func assertFetchMakesTheBlobReadable(test *testing.T, fixture pullRequestFixture) {
	test.Helper()
	ctx := context.Background()
	if _, err := ReadBlob(ctx, fixture.checkout, fixture.identifier); !errors.Is(err, ErrBlobNotFound) {
		test.Fatalf("before the fetch: err = %v, want ErrBlobNotFound", err)
	}
	referencesBefore, fetchHeadBefore := snapshot(test, fixture.checkout)

	if err := FetchPullRequestHead(ctx, fixture.checkout, "origin", 1); err != nil {
		test.Fatal(err)
	}

	got, err := ReadBlob(ctx, fixture.checkout, fixture.identifier)
	if err != nil {
		test.Fatalf("after the fetch: %v", err)
	}
	if string(got) != "only in the pull request\n" {
		test.Errorf("contents = %q", got)
	}
	referencesAfter, fetchHeadAfter := snapshot(test, fixture.checkout)
	if diff := cmp.Diff(referencesBefore, referencesAfter); diff != "" {
		test.Errorf("references changed (-before +after):\n%s", diff)
	}
	if fetchHeadAfter != fetchHeadBefore {
		test.Errorf("FETCH_HEAD exists = %v after the fetch, was %v", fetchHeadAfter, fetchHeadBefore)
	}
}

func TestFetchPullRequestHeadFailures(test *testing.T) {
	fixture := newPullRequestFixture(test)
	ctx := context.Background()
	for _, testCase := range []struct {
		name       string
		remoteName string
		number     int
	}{
		{"no such pull request", "origin", 2},
		{"no such remote", "elsewhere", 1},
		{"zero number", "origin", 0},
		{"empty remote name", "", 1},
		{"remote name that looks like an option", "--upload-pack=true", 1},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			if err := FetchPullRequestHead(ctx, fixture.checkout, testCase.remoteName, testCase.number); err == nil {
				test.Error("err = nil, want an error")
			}
		})
	}
}

func TestFetchPullRequestHeadStopsWhenCancelled(test *testing.T) {
	fixture := newPullRequestFixture(test)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := FetchPullRequestHead(ctx, fixture.checkout, "origin", 1)
	if !errors.Is(err, context.Canceled) {
		test.Errorf("err = %v, want context.Canceled", err)
	}
}

// lookup returns the value an exec environment gives name: the last entry wins.
func lookup(variables []string, name string) (string, bool) {
	value, found := "", false
	for _, variable := range variables {
		if entryName, entryValue, _ := strings.Cut(variable, "="); entryName == name {
			value, found = entryValue, true
		}
	}
	return value, found
}

func TestFetchCommandArguments(test *testing.T) {
	command := newFetchCommand(context.Background(), "/work", "upstream", 42, nil, "")
	want := []string{
		"git", "fetch", "--no-write-fetch-head", "--refmap=", "--no-tags",
		"--no-auto-maintenance", "--no-recurse-submodules", "--quiet", "--",
		"upstream", "refs/pull/42/head",
	}
	if diff := cmp.Diff(want, command.Args); diff != "" {
		test.Errorf("arguments (-want +got):\n%s", diff)
	}
	if command.Dir != "/work" {
		test.Errorf("Dir = %q, want /work", command.Dir)
	}
	if command.Stdin != nil {
		test.Errorf("Stdin = %v, want nil (the null device)", command.Stdin)
	}
}

func TestFetchEnvironmentDisablesPrompting(test *testing.T) {
	for _, testCase := range []struct {
		name                 string
		base                 []string
		configuredSSHCommand string
		wantSSHCommand       string
		wantSSHCommandSet    bool
	}{
		{"plain", []string{"PATH=/bin"}, "", "ssh -o BatchMode=yes", true},
		{"prompting turned on by the user", []string{"GIT_TERMINAL_PROMPT=1"}, "", "ssh -o BatchMode=yes", true},
		{"configured ssh command is kept", []string{"PATH=/bin"}, "ssh -i /keys/work", "ssh -i /keys/work -o BatchMode=yes", true},
		{"the user's GIT_SSH_COMMAND wins", []string{"GIT_SSH_COMMAND=ssh -v"}, "ssh -i /keys/work", "ssh -v", true},
		{"the user's GIT_SSH is not replaced", []string{"GIT_SSH=/usr/bin/plink"}, "", "", false},
		{"core.sshCommand beats GIT_SSH and gets BatchMode", []string{"GIT_SSH=/usr/bin/plink"}, "ssh -i /keys/work", "ssh -i /keys/work -o BatchMode=yes", true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			command := newFetchCommand(context.Background(), "/work", "origin", 1, testCase.base, testCase.configuredSSHCommand)
			if value, _ := lookup(command.Env, "GIT_TERMINAL_PROMPT"); value != "0" {
				test.Errorf("GIT_TERMINAL_PROMPT = %q, want 0", value)
			}
			value, found := lookup(command.Env, "GIT_SSH_COMMAND")
			if found != testCase.wantSSHCommandSet || value != testCase.wantSSHCommand {
				test.Errorf("GIT_SSH_COMMAND = %q (set %v), want %q (set %v)", value, found, testCase.wantSSHCommand, testCase.wantSSHCommandSet)
			}
			if value, found := lookup(command.Env, "PATH"); testCase.base[0] == "PATH=/bin" && (!found || value != "/bin") {
				test.Errorf("PATH = %q (set %v), want it kept", value, found)
			}
		})
	}
}

func TestFetchEnvironmentDropsTokens(test *testing.T) {
	command := newFetchCommand(context.Background(), "/work", "origin", 1,
		[]string{"GH_TOKEN=secret", "GITHUB_TOKEN=secret", "KEPT=1"}, "")
	if slices.ContainsFunc(command.Env, func(variable string) bool { return strings.Contains(variable, "secret") }) {
		test.Errorf("environment still holds a token: %v", command.Env)
	}
	if _, found := lookup(command.Env, "KEPT"); !found {
		test.Error("KEPT was dropped")
	}
}

// TestReadBlobIgnoresReferencesNamedLikeTheIdentifier: git resolves a short
// name to a branch or tag before an object, so a ref named like the blob's
// abbreviation must not change what is read.
func TestReadBlobIgnoresReferencesNamedLikeTheIdentifier(test *testing.T) {
	for _, testCase := range []struct {
		name   string
		create func(test *testing.T, directory, name, decoy string)
	}{
		{"tag pointing at another blob", func(test *testing.T, directory, name, decoy string) {
			runGit(test, directory, "tag", name, decoy)
		}},
		{"branch", func(test *testing.T, directory, name, _ string) {
			runGit(test, directory, "branch", name)
		}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			directory := newRepository(test)
			identifier := writeBlob(test, directory, []byte("the real blob\n"))
			decoy := writeBlob(test, directory, []byte("decoy\n"))
			short := identifier[:7]
			testCase.create(test, directory, short, decoy)
			got, err := ReadBlob(context.Background(), directory, short)
			if err != nil {
				test.Fatal(err)
			}
			if string(got) != "the real blob\n" {
				test.Errorf("contents = %q, want the real blob", got)
			}
		})
	}
}

// TestReadBlobSkipsOtherObjectTypes: an abbreviation shared by a blob and a
// commit still finds the blob.
func TestReadBlobSkipsOtherObjectTypes(test *testing.T) {
	directory := newRepository(test)
	commit := runGit(test, directory, "rev-parse", "HEAD")
	// Find, in process, contents whose blob name shares the commit's first
	// four digits (about one in 65536), then write only that blob.
	for i := range 1 << 22 {
		contents := "blob " + strconv.Itoa(i) + "\n"
		if blobName(contents)[:4] != commit[:4] {
			continue
		}
		if identifier := writeBlob(test, directory, []byte(contents)); identifier[:4] != commit[:4] {
			test.Fatalf("git named the blob %s, want the prefix %s", identifier, commit[:4])
		}
		got, err := ReadBlob(context.Background(), directory, commit[:4])
		if err != nil {
			test.Fatal(err)
		}
		if string(got) != contents {
			test.Errorf("contents = %q, want %q", got, contents)
		}
		return
	}
	test.Fatal("no contents gave a blob name with the commit's prefix")
}

// blobName is the SHA-1 object name git gives a blob with these contents.
func blobName(contents string) string {
	sum := sha1.Sum([]byte("blob " + strconv.Itoa(len(contents)) + "\x00" + contents))
	return hex.EncodeToString(sum[:])
}
