package clipboard

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type call struct {
	Path        string
	Arguments   []string
	Input       string
	Environment []string
}

func TestCopyChoosesTool(test *testing.T) {
	for _, testCase := range []struct {
		name      string
		platform  string
		variables map[string]string
		installed []string
		want      call
		wantErr   error
	}{
		{"macos", "darwin", nil, []string{"pbcopy", "xclip"}, call{Path: "/bin/pbcopy"}, nil},
		{"wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, []string{"wl-copy", "xclip", "xsel"}, call{Path: "/bin/wl-copy"}, nil},
		{"wayland without wl-copy falls back to xclip", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, []string{"xclip"}, call{Path: "/bin/xclip", Arguments: []string{"-selection", "clipboard"}}, nil},
		{"x11 prefers xclip", "linux", map[string]string{"DISPLAY": ":0"}, []string{"wl-copy", "xclip", "xsel"}, call{Path: "/bin/xclip", Arguments: []string{"-selection", "clipboard"}}, nil},
		{"only xsel", "linux", map[string]string{"DISPLAY": ":0"}, []string{"xsel"}, call{Path: "/bin/xsel", Arguments: []string{"--clipboard", "--input"}}, nil},
		{"no display, no x11 tool", "linux", nil, []string{"xclip", "xsel"}, call{}, ErrNoTool},
		{"wayland without wl-copy ignores x11 tools without DISPLAY", "linux", map[string]string{"WAYLAND_DISPLAY": "w"}, []string{"xclip"}, call{}, ErrNoTool},
		{"none on linux", "linux", nil, nil, call{}, ErrNoTool},
		{"pbcopy is not tried on linux", "linux", nil, []string{"pbcopy"}, call{}, ErrNoTool},
		{"unknown platform", "windows", nil, []string{"xclip", "pbcopy"}, call{}, ErrNoTool},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var got []call
			system := New(Options{
				Platform:       testCase.platform,
				GetEnvironment: func(name string) string { return testCase.variables[name] },
				LookPath: func(name string) (string, error) {
					for _, installed := range testCase.installed {
						if installed == name {
							return "/bin/" + name, nil
						}
					}
					return "", exec.ErrNotFound
				},
				Environment: []string{"A=1"},
				Run: func(command *exec.Cmd) error {
					input, err := io.ReadAll(command.Stdin)
					if err != nil {
						test.Fatal(err)
					}
					got = append(got, call{Path: command.Path, Arguments: command.Args[1:], Input: string(input), Environment: command.Env})
					return nil
				},
			})
			err := system.Copy(context.Background(), "internal/sync/client.go")
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			var want []call
			if testCase.wantErr == nil {
				testCase.want.Input = "internal/sync/client.go"
				testCase.want.Environment = []string{"A=1"}
				want = []call{testCase.want}
			}
			if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
				test.Errorf("calls (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCopyBoundsWaitForForkedChildren(test *testing.T) {
	var delay time.Duration
	system := New(Options{
		Platform:       "darwin",
		GetEnvironment: func(string) string { return "" },
		LookPath:       func(name string) (string, error) { return "/bin/" + name, nil },
		Run:            func(command *exec.Cmd) error { delay = command.WaitDelay; return nil },
	})
	if err := system.Copy(context.Background(), "x"); err != nil {
		test.Fatal(err)
	}
	if delay <= 0 {
		test.Error("WaitDelay not set: a forked child holding stdin could outlast the deadline")
	}
}

func TestCopyToolFailure(test *testing.T) {
	failure := errors.New("exit status 1")
	system := New(Options{
		Platform:       "darwin",
		GetEnvironment: func(string) string { return "" },
		LookPath:       func(name string) (string, error) { return "/bin/" + name, nil },
		Run:            func(*exec.Cmd) error { return failure },
	})
	err := system.Copy(context.Background(), "secret text")
	var toolError *ToolError
	if !errors.As(err, &toolError) || toolError.Tool != "pbcopy" || !errors.Is(err, failure) {
		test.Fatalf("err = %v", err)
	}
	if got, want := err.Error(), "run pbcopy: exit status 1"; got != want {
		test.Errorf("message = %q, want %q (it must not carry the text)", got, want)
	}
}

func TestCopyWithoutLookup(test *testing.T) {
	if err := New(Options{Platform: "darwin"}).Copy(context.Background(), "x"); !errors.Is(err, ErrNoTool) {
		test.Errorf("err = %v, want ErrNoTool", err)
	}
}
