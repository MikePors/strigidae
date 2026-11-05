package browseropener

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/environment"
)

// started is what a test's Run saw.
type started struct {
	Path        string
	Arguments   []string
	Environment []string
	Discarded   bool
}

// fakeSetup describes the machine an Opener runs on.
type fakeSetup struct {
	platform           string
	variables          map[string]string
	configured         string
	programs           []string // names LookPath finds
	files              []string // paths Lstat finds
	processEnvironment []string
}

func (setup fakeSetup) opener(runs *[]started) *Opener {
	return New(Options{
		Platform:          setup.platform,
		GetEnvironment:    func(name string) string { return setup.variables[name] },
		ConfiguredBrowser: func() string { return setup.configured },
		LookPath: func(name string) (string, error) {
			for _, program := range setup.programs {
				if program == name {
					return "/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		Lstat: func(name string) (fs.FileInfo, error) {
			for _, file := range setup.files {
				if file == name {
					return fakeInfo{}, nil
				}
			}
			return nil, os.ErrNotExist
		},
		Environment: environment.Scrub(setup.processEnvironment),
		Run: func(command *exec.Cmd) error {
			*runs = append(*runs, started{
				Path:        command.Path,
				Arguments:   command.Args[1:],
				Environment: command.Env,
				Discarded:   command.Stdout == nil && command.Stderr == nil,
			})
			return nil
		},
	})
}

type fakeInfo struct{ fs.FileInfo }

func TestLauncherResolution(test *testing.T) {
	const address = "https://example.com/pull/1"
	for _, testCase := range []struct {
		name  string
		setup fakeSetup
		want  started
	}{
		{
			name: "GH_BROWSER wins over everything",
			setup: fakeSetup{
				platform: "linux", variables: map[string]string{"GH_BROWSER": "first --flag", "BROWSER": "third"},
				configured: "second", programs: []string{"first", "second", "third", "xdg-open"},
			},
			want: started{Path: "/bin/first", Arguments: []string{"--flag", address}},
		},
		{
			name: "gh config wins over BROWSER",
			setup: fakeSetup{
				platform: "linux", variables: map[string]string{"BROWSER": "third"},
				configured: "second", programs: []string{"second", "third", "xdg-open"},
			},
			want: started{Path: "/bin/second", Arguments: []string{address}},
		},
		{
			name: "BROWSER wins over the platform default",
			setup: fakeSetup{
				platform: "linux", variables: map[string]string{"BROWSER": "third"},
				programs: []string{"third", "xdg-open"},
			},
			want: started{Path: "/bin/third", Arguments: []string{address}},
		},
		{
			name:  "quoted launcher words stay together",
			setup: fakeSetup{platform: "linux", variables: map[string]string{"GH_BROWSER": `sh -c "echo hi"`}, programs: []string{"sh"}},
			want:  started{Path: "/bin/sh", Arguments: []string{"-c", "echo hi", address}},
		},
		{
			name:  "linux default is xdg-open",
			setup: fakeSetup{platform: "linux", programs: []string{"xdg-open", "www-browser"}},
			want:  started{Path: "/bin/xdg-open", Arguments: []string{address}},
		},
		{
			name:  "linux falls back past a missing xdg-open",
			setup: fakeSetup{platform: "linux", programs: []string{"www-browser", "wslview"}},
			want:  started{Path: "/bin/www-browser", Arguments: []string{address}},
		},
		{
			name:  "macOS default is open",
			setup: fakeSetup{platform: "darwin", programs: []string{"open"}},
			want:  started{Path: "/bin/open", Arguments: []string{address}},
		},
		{
			name:  "windows default is rundll32",
			setup: fakeSetup{platform: "windows", programs: []string{"rundll32"}},
			want:  started{Path: "/bin/rundll32", Arguments: []string{"url.dll,FileProtocolHandler", address}},
		},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var runs []started
			if err := testCase.setup.opener(&runs).Open(address); err != nil {
				test.Fatalf("Open: %v", err)
			}
			if len(runs) != 1 {
				test.Fatalf("started %d commands, want 1", len(runs))
			}
			got := runs[0]
			got.Environment, got.Discarded = nil, false
			testCase.want.Environment = nil
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("started command mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLauncherErrors(test *testing.T) {
	for _, testCase := range []struct {
		name  string
		setup fakeSetup
		want  string
	}{
		{"no linux provider", fakeSetup{platform: "linux"}, "xdg-open"},
		{"configured launcher missing", fakeSetup{platform: "linux", variables: map[string]string{"GH_BROWSER": "nosuch"}}, "not found"},
		{"unbalanced quote", fakeSetup{platform: "linux", variables: map[string]string{"GH_BROWSER": `sh "oops`}, programs: []string{"sh"}}, "parse browser launcher"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var runs []started
			err := testCase.setup.opener(&runs).Open("https://example.com")
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				test.Errorf("Open error = %v, want one containing %q", err, testCase.want)
			}
			if len(runs) != 0 {
				test.Errorf("started %d commands, want none", len(runs))
			}
		})
	}
}

func TestRunError(test *testing.T) {
	failure := errors.New("launcher failed")
	opener := New(Options{
		Platform:    "darwin",
		LookPath:    func(name string) (string, error) { return "/bin/" + name, nil },
		Environment: []string{},
		Run:         func(*exec.Cmd) error { return failure },
	})
	if err := opener.Open("https://example.com"); !errors.Is(err, failure) {
		test.Errorf("Open error = %v, want %v", err, failure)
	}
}

func TestAddressChecks(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		address string
		setup   fakeSetup
		want    string // URL opened; empty means rejected
	}{
		{name: "https", address: "https://github.com/a/b/pull/1", want: "https://github.com/a/b/pull/1"},
		{name: "http", address: "http://example.com", want: "http://example.com"},
		{name: "vscode", address: "vscode://file/x", want: "vscode://file/x"},
		{name: "vscode insiders", address: "vscode-insiders://file/x", want: "vscode-insiders://file/x"},
		{name: "no scheme becomes https", address: "example.com/x", want: "https://example.com/x"},
		{name: "file scheme", address: "file:///etc/passwd"},
		{name: "javascript scheme", address: "javascript:alert(1)"},
		{name: "ssh scheme", address: "ssh://host"},
		{name: "unparsable", address: "http://[::1"},
		{name: "existing file", address: "notes.txt", setup: fakeSetup{files: []string{"notes.txt"}}},
		{name: "executable on the path", address: "rm", setup: fakeSetup{programs: []string{"rm"}}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			testCase.setup.platform = "darwin"
			testCase.setup.programs = append(testCase.setup.programs, "open")
			var runs []started
			err := testCase.setup.opener(&runs).Open(testCase.address)
			if testCase.want == "" {
				if err == nil {
					test.Fatalf("Open(%q) succeeded, want an error", testCase.address)
				}
				if len(runs) != 0 {
					test.Errorf("started %d commands for a rejected address", len(runs))
				}
				return
			}
			if err != nil {
				test.Fatalf("Open(%q): %v", testCase.address, err)
			}
			if got := runs[0].Arguments[len(runs[0].Arguments)-1]; got != testCase.want {
				test.Errorf("opened %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestNilEnvironmentIsEmptyNotInherited(test *testing.T) {
	var command *exec.Cmd
	opener := New(Options{
		Platform:    "darwin",
		LookPath:    func(name string) (string, error) { return "/bin/" + name, nil },
		Environment: nil,
		Run:         func(started *exec.Cmd) error { command = started; return nil },
	})
	if err := opener.Open("https://example.com"); err != nil {
		test.Fatalf("Open: %v", err)
	}
	if command.Env == nil || len(command.Env) != 0 {
		test.Errorf("Env = %#v, want non-nil and empty", command.Env)
	}
}

func TestEnvironmentIsScrubbedAndOutputDiscarded(test *testing.T) {
	setup := fakeSetup{
		platform: "darwin",
		programs: []string{"open"},
		processEnvironment: []string{
			"PATH=/usr/bin", "GH_TOKEN=secret", "GITHUB_TOKEN=secret", "GH_ENTERPRISE_TOKEN=secret",
			"GITHUB_ENTERPRISE_TOKEN=secret", "HOME=/home/someone", "GH_HOST=github.com",
		},
	}
	var runs []started
	if err := setup.opener(&runs).Open("https://example.com"); err != nil {
		test.Fatalf("Open: %v", err)
	}
	want := []string{"PATH=/usr/bin", "HOME=/home/someone", "GH_HOST=github.com"}
	if diff := cmp.Diff(want, runs[0].Environment); diff != "" {
		test.Errorf("environment mismatch (-want +got):\n%s", diff)
	}
	if !runs[0].Discarded {
		test.Error("output is not discarded")
	}
}
