// Package atomicfile writes files by replacing them with a complete copy.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces path with data and the given permissions. New parent
// directories are private. The temporary file is synced before the rename.
func Write(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".write-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	name := temporary.Name()
	_, err = temporary.Write(data)
	if err == nil {
		err = temporary.Chmod(mode)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		_ = os.Remove(name) // best effort: the write already failed
		return err
	}
	syncDirectory(directory)
	return nil
}

// syncDirectory makes the rename durable, as far as the file system allows. It
// is best effort: the new file is already in place.
func syncDirectory(directory string) {
	handle, err := os.Open(directory)
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}
