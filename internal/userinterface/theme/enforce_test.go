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
// internal/userinterface/CLAUDE.md) over every non-test Go file under internal/userinterface:
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
	colourConstructors = map[string]map[string]bool{
		lipglossPath: {"Color": true, "ANSIColor": true, "RGBColor": true},
		ansiPath:     {"IndexedColor": true, "BasicColor": true, "TrueColor": true, "RGBColor": true},
	}
	// image/color types a style could be built from directly.
	colourTypes = map[string]bool{
		"RGBA": true, "RGBA64": true, "NRGBA": true, "NRGBA64": true, "Gray": true, "Gray16": true,
		"CMYK": true, "YCbCr": true, "Alpha": true, "Alpha16": true,
	}
	colourConstants = map[string]bool{"Black": true, "White": true, "Transparent": true, "Opaque": true}
)

// violations returns what is wrong with the Go file src at rel, a slash path
// relative to internal/userinterface (such as "theme/roles.go").
func violations(relative, source string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, relative, source, 0)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{} // local name -> import path
	for _, importSpec := range file.Imports {
		path, _ := strconv.Unquote(importSpec.Path.Value)
		name := filepath.Base(path)
		if path == lipglossPath {
			name = "lipgloss"
		}
		if importSpec.Name != nil {
			name = importSpec.Name.Name
		}
		paths[name] = path
	}
	inTheme := strings.HasPrefix(relative, "theme/")
	palettesFile := relative == "theme/palettes.go"
	colourFile := palettesFile || relative == "theme/color.go"
	slotFile := palettesFile || relative == "theme/palette.go" || relative == "theme/roles.go"

	var out []string
	bad := func(node ast.Node, format string, arguments ...any) {
		out = append(out, fmt.Sprintf("%s: %s", fileSet.Position(node.Pos()), fmt.Sprintf(format, arguments...)))
	}
	packageSelection := func(expression ast.Expr) (packageName, selection string, ok bool) {
		selector, isSelection := expression.(*ast.SelectorExpr)
		if !isSelection {
			return "", "", false
		}
		packageIdentifier, isIdentifier := selector.X.(*ast.Ident)
		if !isIdentifier {
			return "", "", false
		}
		return paths[packageIdentifier.Name], selector.Sel.Name, paths[packageIdentifier.Name] != ""
	}

	checkString := func(node ast.Node, text string) {
		if !palettesFile && hexLiteral.MatchString(text) {
			bad(node, "hex colour %q outside theme/palettes.go", text)
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if text, err := strconv.Unquote(node.Value); err == nil {
					checkString(node, text)
				}
			}
		case *ast.BinaryExpr:
			// "#" + "ff8787" and similar constant folding.
			if text, ok := foldStrings(node); ok {
				checkString(node, text)
			}
		case *ast.CallExpr:
			if packageName, selection, ok := packageSelection(node.Fun); ok && colourConstructors[packageName][selection] && !colourFile {
				bad(node, "colour constructor %s.%s outside the palette files", filepath.Base(packageName), selection)
			}
		case *ast.CompositeLit:
			if packageName, selection, ok := packageSelection(node.Type); ok && packageName == colorPath && colourTypes[selection] && relative != "theme/color.go" {
				bad(node, "color.%s literal outside theme/color.go", selection)
			}
		case *ast.SelectorExpr:
			if packageName, selection, ok := packageSelection(node); ok {
				if (packageName == lipglossPath || packageName == ansiPath) && namedColours[selection] {
					bad(node, "named colour %s.%s; use a role", filepath.Base(packageName), selection)
				}
				if packageName == colorPath && colourConstants[selection] && !inTheme {
					bad(node, "color.%s outside the theme package", selection)
				}
			}
			if node.Sel.Name == "colors" && !palettesFile && relative != "theme/palette.go" {
				bad(node, "direct palette slot access (.colors)")
			}
			if node.Sel.Name == "Slot" && !slotFile {
				bad(node, "palette slot read via .Slot; only roles.go may")
			}
		case *ast.Ident:
			if slotName.MatchString(node.Name) && !slotFile {
				bad(node, "palette slot %s used outside roles.go", node.Name)
			}
		}
		return true
	})
	return out, nil
}

// foldStrings evaluates a tree of "+" over string literals.
func foldStrings(expression ast.Expr) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return "", false
		}
		rightText, err := strconv.Unquote(expression.Value)
		return rightText, err == nil
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return "", false
		}
		leftText, leftOk := foldStrings(expression.X)
		rightText, rightOk := foldStrings(expression.Y)
		return leftText + rightText, leftOk && rightOk
	case *ast.ParenExpr:
		return foldStrings(expression.X)
	}
	return "", false
}

// TestEnforcementRules proves each rule fires on a small source snippet, and
// that the allowed places stay allowed.
func TestEnforcementRules(test *testing.T) {
	const imports = `import ("charm.land/lipgloss/v2"; "image/color"; "github.com/charmbracelet/x/ansi")` + "\n"
	tests := []struct {
		name, relative, body, want string // want is a substring of a violation; "" means clean
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			got, err := violations(testCase.relative, "package p\n"+imports+testCase.body+"\n")
			if err != nil {
				test.Fatal(err)
			}
			if testCase.want == "" {
				if len(got) != 0 {
					test.Errorf("unexpected violations: %v", got)
				}
				return
			}
			if !strings.Contains(strings.Join(got, "\n"), testCase.want) {
				test.Errorf("violations = %v, want one containing %q", got, testCase.want)
			}
		})
	}
}

// TestNoColourOutsideThePalette runs the rules over the real tree.
func TestNoColourOutsideThePalette(test *testing.T) {
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel("..", path)
		got, err := violations(filepath.ToSlash(relative), string(bytes))
		if err != nil {
			return err
		}
		for _, value := range got {
			test.Error(value)
		}
		return nil
	})
	if err != nil {
		test.Fatal(err)
	}
}
