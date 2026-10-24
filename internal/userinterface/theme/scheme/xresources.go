package scheme

import (
	"bufio"
	"bytes"
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// IsXresources reports whether a theme path names an Xresources file.
func IsXresources(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".xresources")
}

// ParseXresources reads a terminal palette from an Xresources file: the
// foreground, background and color0 to color15 resources, under any class or
// instance prefix (`*.color0`, `*color0`, `URxvt.color0`). Other resources
// are ignored. Values are "#rrggbb", or the name of a `#define` whose value
// is; a palette resource with any other value is an error, so a file with
// app-specific colours (`XTerm*background: black`) or conditionals needs
// trimming to its palette. A later line wins over an earlier one, however
// specific its prefix. source names the file in errors.
func ParseXresources(source string, data []byte) (Scheme, error) {
	if len(data) > MaximumFileSize {
		return Scheme{}, &Error{Source: source, Message: fmt.Sprintf("theme file is too large (limit %d bytes)", MaximumFileSize)}
	}
	defines := map[string]string{}
	colours := map[string]color.Color{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, MaximumFileSize)
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.HasPrefix(line, "/*") {
			return Scheme{}, &Error{Source: source, Line: number, Message: fmt.Sprintf("/* */ comments are not supported (use ! to comment), got %q", line)}
		}
		if directive, ok := strings.CutPrefix(line, "#"); ok {
			fields := strings.Fields(directive)
			if len(fields) != 3 || fields[0] != "define" {
				return Scheme{}, &Error{Source: source, Line: number, Message: fmt.Sprintf("unsupported preprocessor line %q (only #define NAME VALUE is)", line)}
			}
			defines[fields[1]] = fields[2]
			continue
		}
		resource, value, ok := strings.Cut(line, ":")
		if !ok {
			return Scheme{}, &Error{Source: source, Line: number, Message: fmt.Sprintf("expected \"resource: value\", got %q", line)}
		}
		name := lastComponent(strings.TrimSpace(resource))
		if !paletteResource(name) {
			continue
		}
		value = strings.TrimSpace(value)
		if defined, ok := defines[value]; ok {
			value = defined
		}
		colour, err := theme.ParseHex(value)
		if err != nil {
			return Scheme{}, &Error{Source: source, Line: number, Message: fmt.Sprintf("%s: %v", name, err)}
		}
		colours[name] = colour
	}
	if err := scanner.Err(); err != nil {
		return Scheme{}, &Error{Source: source, Message: err.Error()}
	}
	return xresourcesScheme(source, colours)
}

func xresourcesScheme(source string, colours map[string]color.Color) (Scheme, error) {
	var missing []string
	need := func(name string) color.Color {
		if colours[name] == nil {
			missing = append(missing, name)
		}
		return colours[name]
	}
	background, foreground := need("background"), need("foreground")
	var ansi [16]color.Color
	for i := range ansi {
		ansi[i] = need("color" + strconv.Itoa(i))
	}
	if len(missing) > 0 {
		return Scheme{}, &Error{Source: source, Message: "missing " + strings.Join(missing, ", ")}
	}
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	if name == "" { // ".Xresources": the whole name is the extension
		name = filepath.Base(source)
	}
	palette, err := theme.FromANSI(name, background, foreground, ansi)
	if err != nil {
		return Scheme{}, &Error{Source: source, Message: err.Error()}
	}
	return Scheme{Name: name, Palette: palette}, nil
}

// lastComponent is a resource name's final component, after the last "." or "*".
func lastComponent(resource string) string {
	return resource[strings.LastIndexAny(resource, ".*")+1:]
}

func paletteResource(name string) bool {
	if name == "foreground" || name == "background" {
		return true
	}
	digits, ok := strings.CutPrefix(name, "color")
	index, err := strconv.Atoi(digits)
	return ok && err == nil && index >= 0 && index < 16 && strconv.Itoa(index) == digits
}
