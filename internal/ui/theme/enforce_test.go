package theme

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// These tests enforce the three-layer colour model (see the package doc and
// internal/ui/CLAUDE.md) over every non-test Go file under internal/ui:
//
//   - colour values (hex literals, lipgloss.Color, ANSI indices, colour
//     constants, color.RGBA literals) exist only in the palette files;
//   - only roles.go reads palette slots;
//   - styles are therefore built from roles alone.

const (
	lipglossPath = "charm.land/lipgloss/v2"
	ansiPath     = "github.com/charmbracelet/x/ansi"
	colorPath    = "image/color"
)

var (
	hexLiteral = regexp.MustCompile(`#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{4}|[0-9a-fA-F]{3})\b`)
	slotName   = regexp.MustCompile(`^Base(?:0[0-9A-F]|1[0-7])$`)

	// Named colours in lipgloss and x/ansi (ANSI palette entries).
	namedColours = map[string]bool{
		"Black": true, "Red": true, "Green": true, "Yellow": true, "Blue": true, "Magenta": true, "Cyan": true, "White": true,
		"BrightBlack": true, "BrightRed": true, "BrightGreen": true, "BrightYellow": true,
		"BrightBlue": true, "BrightMagenta": true, "BrightCyan": true, "BrightWhite": true,
	}
	// Colour constructors that take a literal value or an index.
	colourCtors = map[string]map[string]bool{
		lipglossPath: {"Color": true, "ANSIColor": true, "RGBColor": true},
		ansiPath:     {"IndexedColor": true, "BasicColor": true, "TrueColor": true, "RGBColor": true},
	}
	// image/color types a style could be built from directly.
	colourTypes = map[string]bool{
		"RGBA": true, "RGBA64": true, "NRGBA": true, "NRGBA64": true, "Gray": true, "Gray16": true,
		"CMYK": true, "YCbCr": true, "Alpha": true, "Alpha16": true,
	}
	colourConsts = map[string]bool{"Black": true, "White": true, "Transparent": true, "Opaque": true}
)

// violations returns what is wrong with the Go file src at rel, a slash path
// relative to internal/ui (such as "theme/roles.go").
func violations(rel, src string) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{} // local name -> import path
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := filepath.Base(p)
		if p == lipglossPath {
			name = "lipgloss"
		}
		if im.Name != nil {
			name = im.Name.Name
		}
		paths[name] = p
	}
	inTheme := strings.HasPrefix(rel, "theme/")
	palettesFile := rel == "theme/palettes.go"
	colourFile := palettesFile || rel == "theme/color.go"
	slotFile := palettesFile || rel == "theme/palette.go" || rel == "theme/roles.go"

	var out []string
	bad := func(n ast.Node, format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), fmt.Sprintf(format, args...)))
	}
	pkgSel := func(e ast.Expr) (pkg, sel string, ok bool) {
		s, isSel := e.(*ast.SelectorExpr)
		if !isSel {
			return "", "", false
		}
		x, isIdent := s.X.(*ast.Ident)
		if !isIdent {
			return "", "", false
		}
		return paths[x.Name], s.Sel.Name, paths[x.Name] != ""
	}

	checkString := func(n ast.Node, s string) {
		if !palettesFile && hexLiteral.MatchString(s) {
			bad(n, "hex colour %q outside theme/palettes.go", s)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				if s, err := strconv.Unquote(n.Value); err == nil {
					checkString(n, s)
				}
			}
		case *ast.BinaryExpr:
			// "#" + "ff8787" and similar constant folding.
			if s, ok := foldStrings(n); ok {
				checkString(n, s)
			}
		case *ast.CallExpr:
			if pkg, sel, ok := pkgSel(n.Fun); ok && colourCtors[pkg][sel] && !colourFile {
				bad(n, "colour constructor %s.%s outside the palette files", filepath.Base(pkg), sel)
			}
		case *ast.CompositeLit:
			if pkg, sel, ok := pkgSel(n.Type); ok && pkg == colorPath && colourTypes[sel] && rel != "theme/color.go" {
				bad(n, "color.%s literal outside theme/color.go", sel)
			}
		case *ast.SelectorExpr:
			if pkg, sel, ok := pkgSel(n); ok {
				if (pkg == lipglossPath || pkg == ansiPath) && namedColours[sel] {
					bad(n, "named colour %s.%s; use a role", filepath.Base(pkg), sel)
				}
				if pkg == colorPath && colourConsts[sel] && !inTheme {
					bad(n, "color.%s outside the theme package", sel)
				}
			}
			if n.Sel.Name == "colors" && !palettesFile && rel != "theme/palette.go" {
				bad(n, "direct palette slot access (.colors)")
			}
			if n.Sel.Name == "Slot" && !slotFile {
				bad(n, "palette slot read via .Slot; only roles.go may")
			}
		case *ast.Ident:
			if slotName.MatchString(n.Name) && !slotFile {
				bad(n, "palette slot %s used outside roles.go", n.Name)
			}
		}
		return true
	})
	return out, nil
}

// foldStrings evaluates a tree of "+" over string literals.
func foldStrings(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		l, ok1 := foldStrings(e.X)
		r, ok2 := foldStrings(e.Y)
		return l + r, ok1 && ok2
	case *ast.ParenExpr:
		return foldStrings(e.X)
	}
	return "", false
}

// TestEnforcementRules proves each rule fires on a small source snippet, and
// that the allowed places stay allowed.
func TestEnforcementRules(t *testing.T) {
	const imports = `import ("charm.land/lipgloss/v2"; "image/color"; "github.com/charmbracelet/x/ansi")` + "\n"
	tests := []struct {
		name, rel, body, want string // want is a substring of a violation; "" means clean
	}{
		{"hex6 in a view", "views/x/x.go", `var c = "#ff8787"`, "hex colour"},
		{"hex3 in a view", "views/x/x.go", `var c = "#f00"`, "hex colour"},
		{"hex4 in a view", "views/x/x.go", `var c = "#f00f"`, "hex colour"},
		{"hex8 in a view", "views/x/x.go", `var c = "#ff878780"`, "hex colour"},
		{"hex split by concatenation", "views/x/x.go", `var c = "#" + "ff8787"`, "hex colour"},
		{"hex in theme but not palettes", "theme/theme.go", `var c = "#ff8787"`, "hex colour"},
		{"hex in palettes.go", "theme/palettes.go", `var c = "#ff8787"`, ""},
		{"lipgloss.Color hex", "views/x/x.go", `var c = lipgloss.Color("#ff8787")`, "colour constructor"},
		{"lipgloss.Color 3 digits", "views/x/x.go", `var c = lipgloss.Color("#f00")`, "colour constructor"},
		{"lipgloss.Color ANSI index", "views/x/x.go", `var c = lipgloss.Color("9")`, "colour constructor"},
		{"lipgloss.ANSIColor", "views/x/x.go", `var c = lipgloss.ANSIColor(9)`, "colour constructor"},
		{"ansi.IndexedColor", "views/x/x.go", `var c = ansi.IndexedColor(196)`, "colour constructor"},
		{"lipgloss.Color in theme style", "theme/theme.go", `var c = lipgloss.Color(s)`, "colour constructor"},
		{"lipgloss.Color in color.go", "theme/color.go", `var c = lipgloss.Color(s)`, ""},
		{"lipgloss.Red", "views/x/x.go", `var c = lipgloss.Red`, "named colour"},
		{"lipgloss.BrightRed", "views/x/x.go", `var c = lipgloss.BrightRed`, "named colour"},
		{"ansi.Red", "theme/theme.go", `var c = ansi.Red`, "named colour"},
		{"color.RGBA in a view", "views/x/x.go", `var c = color.RGBA{R: 255, A: 255}`, "color.RGBA literal"},
		{"color.NRGBA in a view", "components/x/x.go", `var c = color.NRGBA{R: 255, A: 255}`, "color.NRGBA literal"},
		{"color.RGBA in theme styles", "theme/theme.go", `var c = color.RGBA{R: 255, A: 255}`, "color.RGBA literal"},
		{"color.RGBA in color.go", "theme/color.go", `var c = color.RGBA{R: 255, A: 255}`, ""},
		{"color.Black in a view", "views/x/x.go", `var c = color.Black`, "outside the theme package"},
		{"color.Black in the theme", "theme/palette.go", `var c = color.Black`, ""},
		{"slot constant in a view", "views/x/x.go", `var s = theme.Base0B`, "palette slot"},
		{"slot constant in styles", "theme/theme.go", `var s = Base0B`, "palette slot"},
		{"slot constant in roles", "theme/roles.go", `var s = Base0B`, ""},
		{"slot read in styles", "theme/theme.go", `var c = p.Slot(x)`, "palette slot read"},
		{"slot read in roles", "theme/roles.go", `var c = p.Slot(x)`, ""},
		{"slot index in a view", "views/x/x.go", `var c = p.colors[14]`, "direct palette slot access"},
		{"slot index in roles", "theme/roles.go", `var c = p.colors[14]`, "direct palette slot access"},
		{"slot index in palette.go", "theme/palette.go", `var c = p.colors[14]`, ""},
		{"clean view", "views/x/x.go", `var s = theme.Default().Muted`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := violations(tc.rel, "package p\n"+imports+tc.body+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("unexpected violations: %v", got)
				}
				return
			}
			if !strings.Contains(strings.Join(got, "\n"), tc.want) {
				t.Errorf("violations = %v, want one containing %q", got, tc.want)
			}
		})
	}
}

// TestNoColourOutsideThePalette runs the rules over the real tree.
func TestNoColourOutsideThePalette(t *testing.T) {
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
		rel, _ := filepath.Rel("..", path)
		got, err := violations(filepath.ToSlash(rel), string(b))
		if err != nil {
			return err
		}
		for _, v := range got {
			t.Error(v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
