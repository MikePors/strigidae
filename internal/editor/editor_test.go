package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCommandChoice(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"fallback", nil, []string{"vi", "f.md"}},
		{"editor", map[string]string{"EDITOR": "nano"}, []string{"nano", "f.md"}},
		{"visual wins", map[string]string{"VISUAL": "hx", "EDITOR": "nano"}, []string{"hx", "f.md"}},
		{"blank visual falls through", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, []string{"nano", "f.md"}},
		{"arguments", map[string]string{"EDITOR": "code  -w"}, []string{"code", "-w", "f.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, Command("f.md", env(tt.env)).Args); diff != "" {
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
	s, err := New("draft", env(map[string]string{"EDITOR": cp + " " + src}))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path()); string(b) != "draft" {
		t.Errorf("prefill = %q", b)
	}
	if filepath.Ext(s.Path()) != ".md" {
		t.Errorf("path %q is not .md", s.Path())
	}
	if err := s.Cmd().Run(); err != nil {
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

func TestResultRemovesFileWhenEditorFailed(t *testing.T) {
	s, err := New("", env(nil))
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
