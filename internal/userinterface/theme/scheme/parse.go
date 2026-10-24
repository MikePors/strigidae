package scheme

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// MaximumFileSize is the largest theme file strig will read.
const MaximumFileSize = 1 << 20

const (
	base16Count = 16
	base24Count = 24
)

var (
	slotNames = func() []string {
		names := make([]string, base24Count)
		for index := range names {
			names[index] = fmt.Sprintf("base%02X", index)
		}
		return names
	}()

	// currentKeys are the top-level keys of the current tinted-theming layout
	// (system and palette), plus strig's extensions.
	currentKeys = []string{"system", "name", "slug", "author", "description", "variant", "palette", "syntax", "roles"}
	// flatKeys are the top-level keys of the older flat layout, whose colours
	// are top-level base00 to base17 keys, plus strig's extensions.
	flatKeys = append([]string{"scheme", "author", "variant", "syntax", "roles"}, slotNames...)

	sixDigits = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
)

// Parse reads a theme file. source names the file in errors.
func Parse(source string, data []byte) (Scheme, error) {
	if len(data) > MaximumFileSize {
		return Scheme{}, &Error{Source: source, Message: fmt.Sprintf("theme file is too large (limit %d bytes)", MaximumFileSize)}
	}
	root, failure := decodeSingle(source, data)
	if failure != nil {
		return Scheme{}, failure
	}
	reader := &reader{source: source}
	reader.rejectAliases(root)
	if reader.err != nil {
		return Scheme{}, reader.err
	}
	scheme := reader.scheme(root)
	if reader.err != nil {
		return Scheme{}, reader.err
	}
	return scheme, nil
}

func decodeSingle(source string, data []byte) (*yaml.Node, *Error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &Error{Source: source, Message: "the theme file is empty"}
		}
		return nil, &Error{Source: source, Message: "invalid YAML: " + strings.TrimPrefix(err.Error(), "yaml: ")}
	}
	var second yaml.Node
	if err := decoder.Decode(&second); !errors.Is(err, io.EOF) {
		return nil, &Error{Source: source, Message: "the theme file must contain a single YAML document"}
	}
	if len(document.Content) == 0 {
		return nil, &Error{Source: source, Message: "the theme file is empty"}
	}
	return document.Content[0], nil
}

// reader records the first error; callers check err once at the end.
type reader struct {
	source string
	err    *Error
}

func (reader *reader) fail(node *yaml.Node, format string, arguments ...any) {
	if reader.err == nil {
		reader.err = &Error{Source: reader.source, Line: node.Line, Message: fmt.Sprintf(format, arguments...)}
	}
}

// rejectAliases fails on anchors, aliases and merge keys anywhere in node.
func (reader *reader) rejectAliases(node *yaml.Node) {
	if reader.err != nil {
		return
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Tag == "!!merge" {
		reader.fail(node, "YAML aliases and merge keys are not supported")
		return
	}
	for _, child := range node.Content {
		reader.rejectAliases(child)
	}
}

// table is a mapping's values by key, in file order.
type table struct {
	keys  []string
	nodes map[string]*yaml.Node
	names map[string]*yaml.Node // the key nodes, for errors
}

// mapping reads a mapping node, rejecting unknown and duplicate keys. what
// names the mapping in errors; noun is "key" or "role".
func (reader *reader) mapping(node *yaml.Node, what, noun string, allowed []string) table {
	found := table{nodes: map[string]*yaml.Node{}, names: map[string]*yaml.Node{}}
	if node.Kind != yaml.MappingNode {
		reader.fail(node, "%s must be a mapping of keys to values", what)
		return found
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode, valueNode := node.Content[index], node.Content[index+1]
		switch {
		case keyNode.Kind != yaml.ScalarNode:
			reader.fail(keyNode, "keys in %s must be plain text", what)
		case !slices.Contains(allowed, keyNode.Value):
			reader.fail(keyNode, "unknown %s %q in %s (valid %ss: %s)", noun, keyNode.Value, what, noun, strings.Join(allowed, ", "))
		case found.nodes[keyNode.Value] != nil:
			reader.fail(keyNode, "duplicate %s %q in %s", noun, keyNode.Value, what)
		}
		if reader.err != nil {
			return found
		}
		found.keys = append(found.keys, keyNode.Value)
		found.nodes[keyNode.Value] = valueNode
		found.names[keyNode.Value] = keyNode
	}
	return found
}

func (reader *reader) text(node *yaml.Node, what string) string {
	value := strings.TrimSpace(node.Value)
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" || value == "" {
		reader.fail(node, "%s must be a non-empty text value", what)
	}
	return value
}

// colour reads a six-digit hex colour, with or without a leading "#".
func (reader *reader) colour(node *yaml.Node, what string) color.Color {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		reader.fail(node, "%s has no value (quote colours that start with #)", what)
		return nil
	}
	text := strings.TrimPrefix(strings.TrimSpace(node.Value), "#")
	if node.Kind != yaml.ScalarNode || !sixDigits.MatchString(text) {
		reader.fail(node, "%s: invalid colour %q: want six hex digits (rrggbb), with or without a leading hash", what, node.Value)
		return nil
	}
	colour, err := theme.ParseHex("#" + text)
	if err != nil {
		reader.fail(node, "%s: %v", what, err)
	}
	return colour
}

// isCurrentLayout reports whether the file is in the current tinted-theming
// layout: it has a palette or a system, which the flat layout never has. A
// misspelled palette is then reported as an unknown key, not blamed on system.
func isCurrentLayout(root *yaml.Node) bool {
	for index := 0; root.Kind == yaml.MappingNode && index+1 < len(root.Content); index += 2 {
		if key := root.Content[index].Value; key == "palette" || key == "system" {
			return true
		}
	}
	return false
}

func (reader *reader) scheme(root *yaml.Node) Scheme {
	current := isCurrentLayout(root)
	allowed := flatKeys
	if current {
		allowed = currentKeys
	}
	top := reader.mapping(root, "the theme file", "key", allowed)
	colourTable, where, system := top, "the theme file", ""
	if current && reader.err == nil && top.nodes["palette"] == nil {
		reader.fail(root, "the theme file is missing palette")
	}
	if current && reader.err == nil {
		colourTable = reader.mapping(top.nodes["palette"], "the palette", "key", slotNames)
		where = "the palette"
		if node := top.nodes["system"]; node != nil {
			system = reader.text(node, "system")
		}
	}
	if reader.err != nil {
		return Scheme{}
	}
	slots := reader.colours(root, top, colourTable, system, where)
	variant, hasVariant := reader.variant(top)
	if reader.err != nil {
		return Scheme{}
	}
	if !hasVariant {
		variant = theme.VariantOf(slots[0])
	}
	name := reader.name(top)
	palette, err := buildPalette(name, variant, slots)
	if err != nil {
		reader.fail(root, "%v", err)
		return Scheme{}
	}
	scheme := Scheme{Name: name, Palette: palette}
	if node := top.nodes["syntax"]; node != nil {
		scheme.Syntax = reader.text(node, "syntax")
		if reader.err == nil {
			if err := theme.CheckSyntaxStyle(scheme.Syntax); err != nil {
				reader.fail(node, "%v", err)
			}
		}
	}
	scheme.Overrides = reader.roles(top.nodes["roles"])
	if reader.err != nil {
		return Scheme{}
	}
	return scheme
}

// colours reads the palette colours and checks that exactly the slots of the
// scheme's system are present. system may be empty, and is then inferred from
// whether any base10 to base17 colour is present. The result is in slot order;
// the last eight are nil for Base16.
func (reader *reader) colours(root *yaml.Node, top, colourTable table, system, where string) [base24Count]color.Color {
	var slots [base24Count]color.Color
	if system == "" {
		system = "base16"
		for _, name := range slotNames[base16Count:] {
			if colourTable.nodes[name] != nil {
				system = "base24"
			}
		}
	}
	count := base16Count
	switch system {
	case "base16":
	case "base24":
		count = base24Count
	default:
		reader.fail(top.nodes["system"], "system %q is not supported (want base16 or base24)", system)
		return slots
	}
	for index, name := range slotNames {
		node := colourTable.nodes[name]
		switch {
		case node == nil && index < count:
			reader.fail(root, "%s is missing %s (a %s scheme needs base00 to %s)", where, name, system, slotNames[count-1])
		case node != nil && index >= count:
			reader.fail(colourTable.names[name], "%s has %s, but the scheme is %s (base00 to %s)", where, name, system, slotNames[count-1])
		case node != nil:
			slots[index] = reader.colour(node, name)
		}
		if reader.err != nil {
			return slots
		}
	}
	return slots
}

func buildPalette(name string, variant theme.Variant, slots [base24Count]color.Color) (theme.Palette, error) {
	if slots[base16Count] != nil {
		return theme.FromBase24(name, variant, slots)
	}
	var base16 [base16Count]color.Color
	copy(base16[:], slots[:])
	return theme.FromBase16(name, variant, base16)
}

// name is the scheme's name, falling back to the file name.
func (reader *reader) name(top table) string {
	for _, key := range []string{"name", "scheme", "slug"} {
		if node := top.nodes[key]; node != nil {
			return reader.text(node, key)
		}
	}
	return strings.TrimSuffix(filepath.Base(reader.source), filepath.Ext(reader.source))
}

func (reader *reader) variant(top table) (theme.Variant, bool) {
	node := top.nodes["variant"]
	if node == nil {
		return theme.Dark, false
	}
	switch value := reader.text(node, "variant"); value {
	case "dark":
		return theme.Dark, true
	case "light":
		return theme.Light, true
	default:
		reader.fail(node, "variant %q is not one of: dark, light", value)
		return theme.Dark, false
	}
}

func (reader *reader) roles(node *yaml.Node) theme.Roles {
	var overrides theme.Roles
	if node == nil {
		return overrides
	}
	roles := reader.mapping(node, "roles", "role", theme.RoleNames())
	for _, name := range roles.keys {
		if reader.err != nil {
			break
		}
		if colour := reader.colour(roles.nodes[name], "roles: "+name); colour != nil {
			if err := overrides.Set(name, colour); err != nil {
				reader.fail(roles.names[name], "%v", err)
			}
		}
	}
	return overrides
}
