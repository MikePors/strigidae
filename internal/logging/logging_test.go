package logging

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPath(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	tests := []struct {
		name    string
		env     map[string]string
		home    func() (string, error)
		want    string
		wantErr bool
	}{
		{"xdg", map[string]string{"XDG_STATE_HOME": "/state"}, home, "/state/strig/strig.log", false},
		{"fallback", nil, home, "/home/u/.local/state/strig/strig.log", false},
		{"relative xdg ignored", map[string]string{"XDG_STATE_HOME": "rel"}, home, "/home/u/.local/state/strig/strig.log", false},
		{"no home", nil, func() (string, error) { return "", errors.New("none") }, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Path(func(k string) string { return tt.env[k] }, tt.home)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "strig.log")
	l, f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Info("hello")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		t.Fatalf("log file empty or unreadable: %v", err)
	}
}
