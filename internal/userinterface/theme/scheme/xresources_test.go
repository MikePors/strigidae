package scheme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestParseXresources(test *testing.T) {
	data, err := os.ReadFile("testdata/tokyo-night.Xresources")
	if err != nil {
		test.Fatal(err)
	}
	scheme, err := ParseXresources("testdata/tokyo-night.Xresources", data)
	if err != nil {
		test.Fatal(err)
	}
	if scheme.Name != "tokyo-night" {
		test.Errorf("name = %q, want the file name", scheme.Name)
	}
	built := scheme.Theme()
	checks := []struct {
		role string
		got  string
		want string
	}{
		{"background", theme.HexString(built.Roles.Background), "#1a1b26"},
		{"text", theme.HexString(built.Roles.Text), "#c0caf5"},
		{"removed (color1)", theme.HexString(built.Roles.Removed), "#f7768e"},
		{"added (color2)", theme.HexString(built.Roles.Added), "#9ece6a"},
		{"accent (color5)", theme.HexString(built.Roles.Accent), "#bb9af7"},
	}
	for _, check := range checks {
		if check.got != check.want {
			test.Errorf("%s = %s, want %s", check.role, check.got, check.want)
		}
	}
	if !built.Chosen || built.FromTerminal || !built.IsDark {
		test.Errorf("Chosen %v, FromTerminal %v, IsDark %v; want true, false, true", built.Chosen, built.FromTerminal, built.IsDark)
	}
}

func TestParseXresourcesErrors(test *testing.T) {
	full := "*.foreground: #c0caf5\n*.background: #1a1b26\n"
	for _, name := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"} {
		full += "*.color" + name + ": #101010\n"
	}
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{"complete", full, ""},
		{"missing colours are named", "*.foreground: #c0caf5\n*.color3: #101010\n", "missing background, color0, color1, color2, color4"},
		{"malformed colour names the line", full + "*.color4: blue\n", "a.Xresources:19: color4"},
		{"undefined name", full + "*.color4: nope\n", "invalid colour \"nope\""},
		{"other preprocessor lines", "#include \"x\"\n" + full, "a.Xresources:1: unsupported preprocessor line"},
		{"line without a colon", full + "junk\n", "a.Xresources:19: expected"},
		{"colour16 is not a palette resource", full + "*.color16: blue\n", ""},
		{"leading zero is not color1", full + "*.color01: blue\n", ""},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			_, err := ParseXresources("a.Xresources", []byte(testCase.data))
			if testCase.wantErr == "" {
				if err != nil {
					test.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				test.Errorf("err = %v, want one containing %q", err, testCase.wantErr)
			}
		})
	}
}

func TestXresourcesNameFromDotFile(test *testing.T) {
	data, err := os.ReadFile("testdata/tokyo-night.Xresources")
	if err != nil {
		test.Fatal(err)
	}
	scheme, err := ParseXresources("/home/me/.Xresources", data)
	if err != nil || scheme.Name != ".Xresources" {
		test.Errorf("name = %q, %v; want .Xresources", scheme.Name, err)
	}
}

func TestResolveXresources(test *testing.T) {
	directory := test.TempDir()
	data, err := os.ReadFile("testdata/tokyo-night.Xresources")
	if err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "mine.xresources"), data, 0o600); err != nil {
		test.Fatal(err)
	}
	scheme, err := Resolve("mine.xresources", directory, fakeHome)
	if err != nil || scheme.Name != "mine" {
		test.Fatalf("Resolve(Xresources file) = %q, %v", scheme.Name, err)
	}
}
