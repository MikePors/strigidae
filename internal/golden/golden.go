// Package golden compares rendered output against testdata/*.golden files.
// It is for tests only; regenerate files with `UPDATE_GOLDEN=1 go test ./...`.
package golden

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

// Assert compares got with testdata/<name>.golden. ANSI escapes are stripped
// first unless keepANSI is set (for tests that are about styling).
func Assert(test *testing.T, name, got string, keepANSI bool) {
	test.Helper()
	if !keepANSI {
		got = ansi.Strip(got)
	}
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			test.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			test.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		test.Fatalf("read golden file (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if diff := cmp.Diff(string(want), got); diff != "" {
		test.Errorf("%s mismatch (-want +got):\n%s", path, diff)
	}
}
