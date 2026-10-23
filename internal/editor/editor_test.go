package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func environment(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCommandChoice(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []string{"sh", "-c", tt.want + ` "$@"`, "strig", "f.md"}
			if diff := cmp.Diff(want, Command("f.md", environment(tt.environment)).Args); diff != "" {
				t.Errorf("args (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSessionRoundTrip(t *testing.T) {
	cp, err := exec.LookPath("cp")
	if err != nil {
		t.Skip("cp not available")
	}
	src := filepath.Join(t.TempDir(), "src.md")
	if err := os.WriteFile(src, []byte("edited body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// "cp src <file>" stands in for an editor that replaces the content.
	s, err := New("draft", environment(map[string]string{"EDITOR": cp + " " + src}))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path()); string(b) != "draft" {
		t.Errorf("prefill = %q", b)
	}
	if filepath.Ext(s.Path()) != ".md" {
		t.Errorf("path %q is not .md", s.Path())
	}
	if err := s.Command().Run(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Result()
	if err != nil || got != "edited body\n" {
		t.Errorf("Result = %q, %v", got, err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Errorf("temporary file not removed: %v", err)
	}
}

func TestQuotedEditorAndPathWithSpaces(t *testing.T) {
	cp, err := exec.LookPath("cp")
	if err != nil {
		t.Skip("cp not available")
	}
	dir := filepath.Join(t.TempDir(), "with space")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src.md")
	if err := os.WriteFile(src, []byte("quoted"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New("", environment(map[string]string{"VISUAL": `"` + cp + `" "` + src + `"`}))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Command().Run(); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Result(); err != nil || got != "quoted" {
		t.Errorf("Result = %q, %v", got, err)
	}
}

func TestResultRemovesFileWhenEditorFailed(t *testing.T) {
	s, err := New("", environment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Result(); err != nil || got != "" {
		t.Errorf("Result = %q, %v", got, err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Error("file not removed")
	}
}
