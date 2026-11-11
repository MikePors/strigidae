package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikePors/strigidae/internal/pullrequestmarks"
)

func TestReadMarks(test *testing.T) {
	corrupt := test.TempDir()
	path := pullrequestmarks.Path(corrupt)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		test.Fatal(err)
	}
	tests := []struct {
		name      string
		directory string
		wantError bool
	}{
		{"no state directory keeps marks for the session", "", false},
		{"no file holds no marks", test.TempDir(), false},
		{"an unreadable file is an error", corrupt, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			marks, err := readMarks(testCase.directory)
			if (err != nil) != testCase.wantError {
				test.Errorf("error = %v, want error %v", err, testCase.wantError)
			}
			if names := marks.Names(); len(names) != 0 {
				test.Errorf("marks = %v", names)
			}
		})
	}
}
