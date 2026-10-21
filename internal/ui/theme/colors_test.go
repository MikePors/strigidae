package theme

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hexLiteral = regexp.MustCompile(`#[0-9a-fA-F]{6}\b`)

// TestHexOnlyInColors keeps every colour value in colors.go so the palette
// has one home.
func TestHexOnlyInColors(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f == "colors.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := hexLiteral.FindString(string(b)); m != "" {
			t.Errorf("%s contains the hex literal %s; move it to colors.go", f, m)
		}
	}
}
