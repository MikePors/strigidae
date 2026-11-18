package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteReplacesFile(test *testing.T) {
	path := filepath.Join(test.TempDir(), "private", "state")
	oldData := bytes.Repeat([]byte("old"), 65536)
	newData := []byte("new")
	if err := Write(path, oldData, 0o600); err != nil {
		test.Fatal(err)
	}
	// A hard link keeps the old file accessible even after path is replaced.
	previous := filepath.Join(filepath.Dir(path), "previous")
	if err := os.Link(path, previous); err != nil {
		test.Fatal(err)
	}
	if err := Write(path, newData, 0o640); err != nil {
		test.Fatal(err)
	}
	data, err := os.ReadFile(previous)
	if err != nil || !bytes.Equal(data, oldData) {
		test.Fatalf("old reader got %d bytes, error %v", len(data), err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(data, newData) {
		test.Fatalf("new reader got %q, error %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		test.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		test.Errorf("permissions = %v, want 0640", info.Mode().Perm())
	}
}

func TestFailedRenameCleansUp(test *testing.T) {
	directory := test.TempDir()
	target := filepath.Join(directory, "state")
	oldFile := filepath.Join(target, "old")
	if err := Write(oldFile, []byte("saved"), 0o600); err != nil {
		test.Fatal(err)
	}
	// Renaming a file over a populated directory fails even as root.
	if err := Write(target, []byte("replacement"), 0o600); err == nil {
		test.Fatal("expected rename to fail")
	}
	data, err := os.ReadFile(oldFile)
	if err != nil || string(data) != "saved" {
		test.Fatalf("old file contains %q, error %v", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		test.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state" {
		test.Errorf("unexpected files after failed rename: %v", entries)
	}
}
