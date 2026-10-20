// Package golden compares rendered output against testdata/*.golden files.
// It is for tests only; regenerate files with `go test ./... -update`.
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Assert compares got with testdata/<name>.golden. ANSI escapes are stripped
// first unless keepANSI is set (for tests that are about styling).
func Assert(t *testing.T, name, got string, keepANSI bool) {
	t.Helper()
	if !keepANSI {
		got = ansi.Strip(got)
	}
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run with -update to create): %v", err)
	}
	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", path, diff)
	}
}
