package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func lookup(entries map[string]string) func(string) string {
	return func(key string) string { return entries[key] }
}

func TestCommandChoice(test *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		want        string // the editor string handed to the shell
	}{
		{"fallback", nil, "vi"},
		{"editor", map[string]string{"EDITOR": "nano"}, "nano"},
		{"visual wins", map[string]string{"VISUAL": "hx", "EDITOR": "nano"}, "hx"},
		{"blank visual falls through", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, "nano"},
		{"arguments and quotes", map[string]string{"EDITOR": "vim -c 'set tw=72'"}, "vim -c 'set tw=72'"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			want := []string{"sh", "-c", testCase.want + ` "$@"`, "strig", "f.md"}
			if diff := cmp.Diff(want, Command("f.md", lookup(testCase.environment)).Args); diff != "" {
				test.Errorf("args (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSessionRoundTrip(test *testing.T) {
	copyPath, err := exec.LookPath("cp")
	if err != nil {
		test.Skip("cp not available")
	}
	source := filepath.Join(test.TempDir(), "src.md")
	if err := os.WriteFile(source, []byte("edited body\n"), 0o600); err != nil {
		test.Fatal(err)
	}
	// "cp src <file>" stands in for an editor that replaces the content.
	session, err := New("draft", lookup(map[string]string{"EDITOR": copyPath + " " + source}))
	if err != nil {
		test.Fatal(err)
	}
	if bytes, _ := os.ReadFile(session.Path()); string(bytes) != "draft" {
		test.Errorf("prefill = %q", bytes)
	}
	if filepath.Ext(session.Path()) != ".md" {
		test.Errorf("path %q is not .md", session.Path())
	}
	if err := session.Command().Run(); err != nil {
		test.Fatal(err)
	}
	got, err := session.Result()
	if err != nil || got != "edited body\n" {
		test.Errorf("Result = %q, %v", got, err)
	}
	if _, err := os.Stat(session.Path()); !os.IsNotExist(err) {
		test.Errorf("temporary file not removed: %v", err)
	}
}

func TestQuotedEditorAndPathWithSpaces(test *testing.T) {
	copyPath, err := exec.LookPath("cp")
	if err != nil {
		test.Skip("cp not available")
	}
	directory := filepath.Join(test.TempDir(), "with space")
	if err := os.Mkdir(directory, 0o700); err != nil {
		test.Fatal(err)
	}
	source := filepath.Join(directory, "src.md")
	if err := os.WriteFile(source, []byte("quoted"), 0o600); err != nil {
		test.Fatal(err)
	}
	session, err := New("", lookup(map[string]string{"VISUAL": `"` + copyPath + `" "` + source + `"`}))
	if err != nil {
		test.Fatal(err)
	}
	if err := session.Command().Run(); err != nil {
		test.Fatal(err)
	}
	if got, err := session.Result(); err != nil || got != "quoted" {
		test.Errorf("Result = %q, %v", got, err)
	}
}

func TestResultRemovesFileWhenEditorFailed(test *testing.T) {
	session, err := New("", lookup(nil))
	if err != nil {
		test.Fatal(err)
	}
	if got, err := session.Result(); err != nil || got != "" {
		test.Errorf("Result = %q, %v", got, err)
	}
	if _, err := os.Stat(session.Path()); !os.IsNotExist(err) {
		test.Error("file not removed")
	}
}

func TestCommandScrubsTokens(test *testing.T) {
	test.Setenv("GH_TOKEN", "secret")
	test.Setenv("GITHUB_TOKEN", "secret")
	test.Setenv("GH_ENTERPRISE_TOKEN", "secret")
	test.Setenv("GITHUB_ENTERPRISE_TOKEN", "secret")
	test.Setenv("STRIG_TEST_KEPT", "kept")

	command := Command("f.md", lookup(nil))

	if command.Env == nil {
		test.Fatal("Env is nil, so the child would inherit every variable")
	}
	var got []string
	for _, variable := range command.Env {
		switch variable {
		case "GH_TOKEN=secret", "GITHUB_TOKEN=secret", "GH_ENTERPRISE_TOKEN=secret", "GITHUB_ENTERPRISE_TOKEN=secret":
			test.Errorf("token variable %q reached the editor", variable)
		case "STRIG_TEST_KEPT=kept":
			got = append(got, variable)
		}
	}
	if diff := cmp.Diff([]string{"STRIG_TEST_KEPT=kept"}, got); diff != "" {
		test.Errorf("other variables must be kept (-want +got):\n%s", diff)
	}
}
