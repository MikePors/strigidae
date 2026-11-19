package atomicfile

import (
	"bytes"
	"errors"
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
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		test.Fatal(err)
	}
	if runtime.GOOS != "windows" && parent.Mode().Perm() != 0o700 {
		test.Errorf("parent permissions = %v, want 0700", parent.Mode().Perm())
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

func TestReadersSeeCompleteFiles(test *testing.T) {
	if runtime.GOOS == "windows" {
		test.Skip("Windows does not guarantee atomic rename over an open file")
	}
	path := filepath.Join(test.TempDir(), "state")
	values := [][]byte{bytes.Repeat([]byte("first"), 32768), bytes.Repeat([]byte("second"), 32768)}
	if err := Write(path, values[0], 0o600); err != nil {
		test.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := range 32 {
			if err := Write(path, values[(index+1)%2], 0o600); err != nil {
				test.Error(err)
				return
			}
		}
	}()
	defer func() { <-done }()
	for {
		data, err := os.ReadFile(path)
		if err != nil {
			test.Fatal(err)
		}
		if !bytes.Equal(data, values[0]) && !bytes.Equal(data, values[1]) {
			test.Fatalf("read incomplete replacement (%d bytes)", len(data))
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

func TestFailedWritePreservesFile(test *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		test.Skip("requires Unix directory permissions enforced for this user")
	}
	directory := test.TempDir()
	path := filepath.Join(directory, "state")
	if err := Write(path, []byte("saved"), 0o600); err != nil {
		test.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		if err := os.Chmod(directory, 0o700); err != nil {
			test.Error(err)
		}
	})
	if err := Write(path, []byte("replacement"), 0o600); !errors.Is(err, os.ErrPermission) {
		test.Fatalf("expected permission error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "saved" {
		test.Fatalf("old file contains %q, error %v", data, err)
	}
}
