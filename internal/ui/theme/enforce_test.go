package theme

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	hexLiteral  = regexp.MustCompile(`#[0-9a-fA-F]{6}\b`)
	lineComment = regexp.MustCompile(`(?m)//.*$`)
	slotUse     = regexp.MustCompile(`\bBase(0[0-9A-F]|1[0-7])\b|\.Slot\(`)
)

// uiSources calls f with the path and text of every non-test Go file under
// internal/ui.
func uiSources(t *testing.T, f func(path, src string)) {
	t.Helper()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f(filepath.ToSlash(path), string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestHexOnlyInPalettes keeps every colour value in the palette definitions.
func TestHexOnlyInPalettes(t *testing.T) {
	uiSources(t, func(path, src string) {
		if path == "../theme/palettes.go" {
			return
		}
		if m := hexLiteral.FindString(src); m != "" {
			t.Errorf("%s contains the hex literal %s; colours belong in theme/palettes.go", path, m)
		}
	})
}

// TestSlotsOnlyReadByRoles keeps styles from reaching past roles to palette
// slots. Only roles.go reads slots; palette.go and palettes.go define them.
func TestSlotsOnlyReadByRoles(t *testing.T) {
	allowed := map[string]bool{
		"../theme/roles.go":    true,
		"../theme/palette.go":  true,
		"../theme/palettes.go": true,
	}
	uiSources(t, func(path, src string) {
		if allowed[path] {
			return
		}
		if m := slotUse.FindString(lineComment.ReplaceAllString(src, "")); m != "" {
			t.Errorf("%s uses palette slot %q; build styles from roles (roles.go)", path, m)
		}
	})
}
