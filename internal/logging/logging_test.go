package logging

import (
	"bytes"
	"errors"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	tests := []struct {
		name        string
		environment map[string]string
		home        func() (string, error)
		want        string
		wantErr     bool
	}{
		{"xdg", map[string]string{"XDG_STATE_HOME": "/state"}, home, "/state/strig/strig.log", false},
		{"fallback", nil, home, "/home/u/.local/state/strig/strig.log", false},
		{"relative xdg ignored", map[string]string{"XDG_STATE_HOME": "rel"}, home, "/home/u/.local/state/strig/strig.log", false},
		{"no home", nil, func() (string, error) { return "", errors.New("none") }, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Path(func(k string) string { return tt.environment[k] }, tt.home)
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

func TestStart(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	home := func() (string, error) { return dir, nil }
	tests := []struct {
		name        string
		environment map[string]string
		home        func() (string, error)
		wantFile    string // log file that should receive the line; "" means discarded
		wantErr     string // prefix of the returned error; "" means no error
	}{
		{
			name: "opens the file", environment: map[string]string{"XDG_STATE_HOME": dir}, home: home,
			wantFile: filepath.Join(dir, "strig", "strig.log"),
		},
		{
			name: "no home", home: func() (string, error) { return "", errors.New("none") },
			wantErr: "find home directory: none",
		},
		{
			name: "unwritable directory", environment: map[string]string{"XDG_STATE_HOME": blocker}, home: home,
			wantErr: "create log directory: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The logger and close must be usable whether or not Start failed.
			l, closeLog, err := Start(func(k string) string { return tt.environment[k] }, tt.home)
			l.Info("hello")
			closeLog()

			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("unexpected error %v", err)
			case tt.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), tt.wantErr)):
				t.Errorf("err = %v, want prefix %q", err, tt.wantErr)
			}
			if tt.wantFile == "" {
				return
			}
			b, err := os.ReadFile(tt.wantFile)
			if err != nil || !strings.Contains(string(b), `msg=hello`) {
				t.Errorf("log file missing line: %q, %v", b, err)
			}
		})
	}
}

func TestInstall(t *testing.T) {
	prevSlog, prevOut, prevFlags := slog.Default(), log.Writer(), log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(prevSlog)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})

	var buf bytes.Buffer
	Install(slog.New(slog.NewTextHandler(&buf, nil)))
	log.Printf("from std log")
	slog.Info("from slog")
	out := buf.String()
	for _, want := range []string{`msg="from std log"`, `msg="from slog"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %s: %q", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(line, "time=") || !strings.Contains(line, "level=") {
			t.Errorf("line lacks timestamp or level: %q", line)
		}
	}
}
