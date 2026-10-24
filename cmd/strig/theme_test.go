package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/scheme"
	"github.com/MikePors/strigidae/internal/userinterface/theme/terminalpalette"
)

func TestRunListThemes(test *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--list-themes"}, &stdout, &stderr, newHarness(nil, false, nil).dependencies); code != 0 {
		test.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr.String())
	}
	want := strings.Join(scheme.Bundled(), "\n") + "\nterminal\n"
	if stdout.String() != want {
		test.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	for _, slug := range []string{"tokyo-night", "catppuccin-latte", "terminal"} {
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

	xresources, err := os.ReadFile("../../internal/userinterface/theme/scheme/testdata/tokyo-night.Xresources")
	if err != nil {
		test.Fatal(err)
	}
	write("palette.Xresources", string(xresources))
	xresourcesConfig := write("xresources.yml", "theme: palette.Xresources\n")

	tests := []struct {
		name         string
		config       string
		wantScheme   string // the scheme's name, "" for the built-in theme
		wantTerminal bool
		wantStderr   string
	}{
		{"unset means built-in", unsetConfig, "", false, ""},
		{"bundled slug", slugConfig, "Catppuccin Latte", false, ""},
		{"file beside the config", fileConfig, "Mine", false, ""},
		{"Xresources file beside the config", xresourcesConfig, "palette", false, ""},
		{"terminal is asked when the screen starts", terminalConfig, "", true, ""},
		{"unknown slug lists bundled themes", unknownConfig, "", false, "tokyo-night"},
		{"missing file", missingConfig, "", false, "gone.yaml"},
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
			if started.terminalTheme != testCase.wantTerminal {
				test.Errorf("terminalTheme = %v, want %v", started.terminalTheme, testCase.wantTerminal)
			}
		})
	}
}

func TestBuildTheme(test *testing.T) {
	var logged bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logged, nil))

	if built, notice := (application{}).buildTheme(logger, neverAsked(test)); built.Chosen || notice != "" {
		test.Errorf("the built-in theme is marked Chosen (%v) or has a notice %q", built.Chosen, notice)
	}

	loaded, err := scheme.Parse("a.yaml", []byte("palette:\n"+greyPalette+"syntax: tokyonight-night\n"))
	if err != nil {
		test.Fatal(err)
	}
	built, _ := (application{scheme: &loaded}).buildTheme(logger, neverAsked(test))
	if !built.Chosen {
		test.Error("a configured theme is not marked Chosen")
	}
	if !strings.Contains(logged.String(), "tokyonight-night") {
		test.Errorf("the ignored syntax: was not logged: %q", logged.String())
	}
}

func neverAsked(test *testing.T) func() (terminalpalette.Reply, error) {
	return func() (terminalpalette.Reply, error) {
		test.Error("the terminal was asked for its palette without theme: terminal")
		return terminalpalette.Reply{}, nil
	}
}

func TestBuildTerminalTheme(test *testing.T) {
	full := "\x1b]10;rgb:c0/ca/f5\a\x1b]11;rgb:1a/1b/26\a"
	for i := range 16 {
		full += fmt.Sprintf("\x1b]4;%d;rgb:%02x/40/40\a", i, i*16)
	}
	full += "\x1b[?62c"
	tests := []struct {
		name       string
		reply      terminalpalette.Reply
		err        error
		wantNotice string // "" when the terminal's palette is used
	}{
		{"full answer", terminalpalette.Parse([]byte(full)), nil, ""},
		{"DA1 only", terminalpalette.Parse([]byte("\x1b[?62c")), nil, "theme terminal: the terminal does not report its colours; using the built-in theme"},
		{"no tty", terminalpalette.Reply{}, errors.New("open terminal: no such device"), "theme terminal: open terminal: no such device; using the built-in theme"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var logged bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logged, nil))
			ask := func() (terminalpalette.Reply, error) { return testCase.reply, testCase.err }
			built, notice := (application{terminalTheme: true}).buildTheme(logger, ask)
			if notice != testCase.wantNotice {
				test.Errorf("notice = %q, want %q", notice, testCase.wantNotice)
			}
			if testCase.wantNotice == "" {
				if !built.Chosen || !built.FromTerminal || theme.HexString(built.Roles.Background) != "#1a1b26" {
					test.Errorf("Chosen %v, FromTerminal %v, background %s; want the terminal's palette", built.Chosen, built.FromTerminal, theme.HexString(built.Roles.Background))
				}
				return
			}
			if built.Chosen || built.Identifier != theme.Default().Identifier {
				test.Error("a failed terminal query did not fall back to the built-in theme")
			}
			if !strings.Contains(logged.String(), "using the built-in theme") {
				test.Errorf("the fallback was not logged: %q", logged.String())
			}
		})
	}
}

const (
	flatRest = "base01: \"202020\"\nbase02: \"303030\"\nbase03: \"707070\"\nbase04: \"909090\"\nbase05: \"c0c0c0\"\nbase06: \"d0d0d0\"\nbase07: \"f0f0f0\"\n" +
		"base08: \"ff0000\"\nbase09: \"ff8800\"\nbase0A: \"ffff00\"\nbase0B: \"00ff00\"\nbase0C: \"00ffff\"\nbase0D: \"0000ff\"\nbase0E: \"ff00ff\"\nbase0F: \"884400\"\n"
	greyPalette = "  base00: \"101010\"\n  base01: \"202020\"\n  base02: \"303030\"\n  base03: \"707070\"\n  base04: \"909090\"\n  base05: \"c0c0c0\"\n  base06: \"d0d0d0\"\n  base07: \"f0f0f0\"\n" +
		"  base08: \"ff0000\"\n  base09: \"ff8800\"\n  base0A: \"ffff00\"\n  base0B: \"00ff00\"\n  base0C: \"00ffff\"\n  base0D: \"0000ff\"\n  base0E: \"ff00ff\"\n  base0F: \"884400\"\n"
)
