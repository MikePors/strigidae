package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
)

func TestRunListThemes(test *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--list-themes"}, &stdout, &stderr, newHarness(nil, false, nil).dependencies); code != 0 {
		test.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	want := strings.Join(scheme.Bundled(), "\n") + "\n"
	if stdout.String() != want {
		test.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	for _, slug := range []string{"tokyo-night", "catppuccin-latte"} {
		if !strings.Contains(stdout.String(), slug+"\n") {
			test.Errorf("--list-themes lacks %s: %q", slug, stdout.String())
		}
	}
}

func TestRunTheme(test *testing.T) {
	directory := test.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			test.Fatal(err)
		}
		return path
	}
	write("mine.yaml", "scheme: Mine\nbase00: \"101010\"\n"+flatRest)
	slugConfig := write("slug.yml", "theme: catppuccin-latte\n")
	fileConfig := write("file.yml", "theme: mine.yaml\n")
	terminalConfig := write("terminal.yml", "theme: terminal\n")
	unknownConfig := write("unknown.yml", "theme: nope\n")
	missingConfig := write("missing.yml", "theme: gone.yaml\n")
	unsetConfig := write("unset.yml", "tabs:\n  - name: A\n")

	tests := []struct {
		name       string
		config     string
		wantScheme string // the scheme's name, "" for the built-in theme
		wantStderr string
	}{
		{"unset means built-in", unsetConfig, "", ""},
		{"bundled slug", slugConfig, "Catppuccin Latte", ""},
		{"file beside the config", fileConfig, "Mine", ""},
		{"terminal is not supported yet", terminalConfig, "", "not supported yet"},
		{"unknown slug lists bundled themes", unknownConfig, "", "tokyo-night"},
		{"missing file", missingConfig, "", "gone.yaml"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			harness := newHarness(nil, false, nil)
			var started *application
			harness.dependencies.start = func(application application, _ io.Writer) error {
				started = &application
				return nil
			}
			var stdout, stderr bytes.Buffer
			code := run([]string{"--config", testCase.config, "-R", "o/r"}, &stdout, &stderr, harness.dependencies)
			if testCase.wantStderr != "" {
				if code != 1 || !strings.Contains(stderr.String(), testCase.wantStderr) || started != nil {
					test.Errorf("code %d, started %v, stderr %q; want a failure mentioning %q before the TUI starts", code, started != nil, stderr.String(), testCase.wantStderr)
				}
				return
			}
			if code != 0 || started == nil {
				test.Fatalf("code %d (stderr %s)", code, stderr.String())
			}
			got := ""
			if started.scheme != nil {
				got = started.scheme.Name
			}
			if diff := cmp.Diff(testCase.wantScheme, got); diff != "" {
				test.Errorf("scheme (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildTheme(test *testing.T) {
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))

	if built := (application{}).buildTheme(logger); built.Chosen {
		test.Error("the built-in theme is marked Chosen")
	}

	loaded, err := scheme.Parse("a.yaml", []byte("palette:\n"+greyPalette+"syntax: tokyonight-night\n"))
	if err != nil {
		test.Fatal(err)
	}
	built := (application{scheme: &loaded}).buildTheme(logger)
	if !built.Chosen {
		test.Error("a configured theme is not marked Chosen")
	}
	if !strings.Contains(logged.String(), "tokyonight-night") {
		test.Errorf("the ignored syntax: was not logged: %q", logged.String())
	}
}

const (
	flatRest = "base01: \"202020\"\nbase02: \"303030\"\nbase03: \"707070\"\nbase04: \"909090\"\nbase05: \"c0c0c0\"\nbase06: \"d0d0d0\"\nbase07: \"f0f0f0\"\n" +
		"base08: \"ff0000\"\nbase09: \"ff8800\"\nbase0A: \"ffff00\"\nbase0B: \"00ff00\"\nbase0C: \"00ffff\"\nbase0D: \"0000ff\"\nbase0E: \"ff00ff\"\nbase0F: \"884400\"\n"
	greyPalette = "  base00: \"101010\"\n  base01: \"202020\"\n  base02: \"303030\"\n  base03: \"707070\"\n  base04: \"909090\"\n  base05: \"c0c0c0\"\n  base06: \"d0d0d0\"\n  base07: \"f0f0f0\"\n" +
		"  base08: \"ff0000\"\n  base09: \"ff8800\"\n  base0A: \"ffff00\"\n  base0B: \"00ff00\"\n  base0C: \"00ffff\"\n  base0D: \"0000ff\"\n  base0E: \"ff00ff\"\n  base0F: \"884400\"\n"
)
