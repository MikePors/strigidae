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

func TestPath(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := Path(func(key string) string { return testCase.environment[key] }, testCase.home)
			if (err != nil) != testCase.wantErr {
				test.Fatalf("err = %v, wantErr %v", err, testCase.wantErr)
			}
			if got != testCase.want {
				test.Errorf("Path() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestOpen(test *testing.T) {
	path := filepath.Join(test.TempDir(), "nested", "strig.log")
	logger, file, err := Open(path)
	if err != nil {
		test.Fatal(err)
	}
	logger.Info("hello")
	if err := file.Close(); err != nil {
		test.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil || len(bytes) == 0 {
		test.Fatalf("log file empty or unreadable: %v", err)
	}
}

func TestStart(test *testing.T) {
	directory := test.TempDir()
	blocker := filepath.Join(directory, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		test.Fatal(err)
	}
	home := func() (string, error) { return directory, nil }
	tests := []struct {
		name        string
		environment map[string]string
		home        func() (string, error)
		wantFile    string // log file that should receive the line; "" means discarded
		wantErr     string // prefix of the returned error; "" means no error
	}{
		{
			name: "opens the file", environment: map[string]string{"XDG_STATE_HOME": directory}, home: home,
			wantFile: filepath.Join(directory, "strig", "strig.log"),
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			// The logger and close must be usable whether or not Start failed.
			logger, closeLog, err := Start(func(key string) string { return testCase.environment[key] }, testCase.home)
			logger.Info("hello")
			closeLog()

			switch {
			case testCase.wantErr == "" && err != nil:
				test.Errorf("unexpected error %v", err)
			case testCase.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), testCase.wantErr)):
				test.Errorf("err = %v, want prefix %q", err, testCase.wantErr)
			}
			if testCase.wantFile == "" {
				return
			}
			bytes, err := os.ReadFile(testCase.wantFile)
			if err != nil || !strings.Contains(string(bytes), `msg=hello`) {
				test.Errorf("log file missing line: %q, %v", bytes, err)
			}
		})
	}
}

func TestInstall(test *testing.T) {
	previousSlog, previousOut, previousFlags := slog.Default(), log.Writer(), log.Flags()
	test.Cleanup(func() {
		slog.SetDefault(previousSlog)
		log.SetOutput(previousOut)
		log.SetFlags(previousFlags)
	})

	var buffer bytes.Buffer
	Install(slog.New(slog.NewTextHandler(&buffer, nil)))
	log.Printf("from std log")
	slog.Info("from slog")
	out := buffer.String()
	for _, want := range []string{`msg="from std log"`, `msg="from slog"`} {
		if !strings.Contains(out, want) {
			test.Errorf("log output missing %s: %q", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(line, "time=") || !strings.Contains(line, "level=") {
			test.Errorf("line lacks timestamp or level: %q", line)
		}
	}
}
