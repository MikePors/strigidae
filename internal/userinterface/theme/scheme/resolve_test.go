package scheme

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/golden"
)

func fakeHome() (string, error) { return "/home/me", nil }

func TestLocate(test *testing.T) {
	tests := []struct {
		value   string
		want    Location
		wantErr string
	}{
		{value: "tokyo-night", want: Location{Slug: "tokyo-night"}},
		{value: "Tokyo Night", want: Location{Slug: "Tokyo Night"}},
		{value: "mine.yaml", want: Location{Path: "/config/mine.yaml"}},
		{value: "mine.YML", want: Location{Path: "/config/mine.YML"}},
		{value: "themes/mine", want: Location{Path: "/config/themes/mine"}},
		{value: "../shared/mine.yml", want: Location{Path: "/shared/mine.yml"}},
		{value: "/abs/mine.yaml", want: Location{Path: "/abs/mine.yaml"}},
		{value: "~/themes/mine.yaml", want: Location{Path: "/home/me/themes/mine.yaml"}},
		{value: "~", want: Location{Slug: "~"}},
		{value: "~mine", want: Location{Slug: "~mine"}},
		{value: "terminal", wantErr: "not supported yet"},
		{value: "~/x.Xresources", wantErr: "not supported yet"},
	}
	for _, testCase := range tests {
		test.Run(testCase.value, func(test *testing.T) {
			got, err := Locate(testCase.value, "/config", fakeHome)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					test.Fatalf("err = %v, want one containing %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				test.Fatal(err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("location mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLocateHomeError(test *testing.T) {
	_, err := Locate("~/a.yaml", "/config", func() (string, error) { return "", errors.New("no home") })
	if err == nil || !strings.Contains(err.Error(), "no home") {
		test.Errorf("err = %v, want the home error", err)
	}
	// Home is not needed unless a ~ is expanded.
	if _, err := Locate("a.yaml", "/config", func() (string, error) { return "", errors.New("no home") }); err != nil {
		test.Errorf("a path without ~ asked for the home directory: %v", err)
	}
}

func TestResolveFileAndSlug(test *testing.T) {
	directory := test.TempDir()
	data, err := os.ReadFile("testdata/base16-flat.yaml")
	if err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "mine.yaml"), data, 0o600); err != nil {
		test.Fatal(err)
	}
	scheme, err := Resolve("mine.yaml", directory, fakeHome)
	if err != nil || scheme.Name != "Solarized Light" {
		test.Fatalf("Resolve(file) = %q, %v", scheme.Name, err)
	}
	if _, err := Resolve("missing.yaml", directory, fakeHome); err == nil {
		test.Error("a missing file is not an error")
	}
	scheme, err = Resolve("tokyo-night", directory, fakeHome)
	if err != nil || scheme.Name != "Tokyo Night" {
		test.Fatalf("Resolve(slug) = %q, %v", scheme.Name, err)
	}
	_, err = Resolve("nope", directory, fakeHome)
	if err == nil || !strings.Contains(err.Error(), "tokyo-night") || !strings.Contains(err.Error(), `"nope"`) {
		test.Errorf("unknown slug error = %v, want it to list the bundled themes", err)
	}
}

func TestBundledListed(test *testing.T) {
	want := []string{"catppuccin-latte", "tokyo-night"}
	if diff := cmp.Diff(want, Bundled()); diff != "" {
		test.Errorf("Bundled() mismatch (-want +got):\n%s", diff)
	}
}

// TestBundledThemesGolden proves every bundled theme parses, and records its
// final roles and any contrast warnings as plain text.
func TestBundledThemesGolden(test *testing.T) {
	for _, slug := range Bundled() {
		test.Run(slug, func(test *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{ReplaceAttr: dropTime})))
			defer slog.SetDefault(previous)

			scheme, err := LoadBundled(slug)
			if err != nil {
				test.Fatal(err)
			}
			built := scheme.Theme()
			if !built.Chosen {
				test.Error("bundled theme is not marked Chosen")
			}
			text := "variant dark=" + map[bool]string{true: "true", false: "false"}[built.IsDark] + "\n" +
				built.Roles.Listing() + "\nwarnings:\n" + logged.String()
			golden.Assert(test, "bundled-"+slug, text, false)
		})
	}
}

func dropTime(_ []string, attribute slog.Attr) slog.Attr {
	if attribute.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attribute
}
