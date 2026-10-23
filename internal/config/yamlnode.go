package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// This file is the generic strict-YAML toolkit the schema in parse.go is
// written with: single document, no aliases, unknown and duplicate keys
// rejected, typed scalar readers, every error located by line.

// Error is a problem in the config file, located by line.
type Error struct {
	Path    string
	Line    int // 1-based; 0 if unknown
	Message string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Message)
	}
	return e.Path + ": " + e.Message
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// decodeSingle parses data as exactly one YAML document and returns its root
// node, or nil if the file has no content.
func decodeSingle(path string, data []byte) (*yaml.Node, *Error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, syntaxError(path, err)
	}
	var second yaml.Node
	switch err := dec.Decode(&second); {
	case err == nil:
		return nil, &Error{Path: path, Line: second.Line, Message: "the config file must contain a single YAML document"}
	case !errors.Is(err, io.EOF):
		return nil, syntaxError(path, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	return &doc, nil
}

// syntaxError converts a yaml.v3 error. The library reports the line of the
// construct it was parsing, which for block-structure mistakes can be the line
// where the enclosing block starts rather than the offending line.
func syntaxError(path string, err error) *Error {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 0
	if m := yamlLine.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
		msg = strings.Replace(msg, m[0]+": ", "", 1)
	}
	return &Error{Path: path, Line: line, Message: "invalid YAML: " + msg}
}

// parser records the first error; callers check err once at the end.
type parser struct {
	path string
	err  *Error
}

func (p *parser) fail(n *yaml.Node, format string, args ...any) {
	if p.err == nil {
		p.err = &Error{Path: p.path, Line: n.Line, Message: fmt.Sprintf(format, args...)}
	}
}

// rejectAliases fails on anchors, aliases and merge keys anywhere in n.
func (p *parser) rejectAliases(n *yaml.Node) {
	if p.err != nil {
		return
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" || n.Tag == "!!merge" {
		p.fail(n, "YAML aliases and merge keys are not supported")
		return
	}
	for _, c := range n.Content {
		p.rejectAliases(c)
	}
}

func keys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// mapping walks the key/value pairs of a mapping node, rejecting unknown and
// duplicate keys. what names the mapping in errors.
func (p *parser) mapping(n *yaml.Node, what string, each func(key string, v *yaml.Node), allowed []string) {
	if n.Kind != yaml.MappingNode {
		p.fail(n, "%s must be a mapping of keys to values", what)
		return
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		switch {
		case !slices.Contains(allowed, k.Value):
			p.fail(k, "unknown key %q in %s (valid keys: %s)", k.Value, what, strings.Join(allowed, ", "))
		case seen[k.Value]:
			p.fail(k, "duplicate key %q in %s", k.Value, what)
		}
		if p.err != nil {
			return
		}
		seen[k.Value] = true
		each(k.Value, v)
		if p.err != nil {
			return
		}
	}
}

// str reads a non-empty scalar as trimmed text; numbers and booleans are
// accepted as written.
func (p *parser) str(n *yaml.Node, what string) string {
	v := strings.TrimSpace(n.Value)
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" || v == "" {
		p.fail(n, "%s must be a non-empty text value", what)
		return ""
	}
	return v
}

// int reads an unquoted integer from lo to hi.
func (p *parser) int(n *yaml.Node, what string, lo, hi int) int {
	v, err := strconv.Atoi(n.Value)
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" || err != nil || v < lo || v > hi {
		p.fail(n, "%s must be a whole number from %d to %d", what, lo, hi)
		return 0
	}
	return v
}

// oneOf reads a text value that must be one of allowed.
func (p *parser) oneOf(n *yaml.Node, what string, allowed []string) string {
	v := p.str(n, what)
	if p.err == nil && !slices.Contains(allowed, v) {
		p.fail(n, "%s %q is not one of: %s", what, v, strings.Join(allowed, ", "))
	}
	return v
}

// uniqueList reads a non-empty sequence, converting each element with item,
// which returns the value and its name. Names must be unique; dupMsg is the
// format (one %q for the name) of the error for a repeat. what names the list
// in the empty-list error. It returns nil if anything failed.
func uniqueList[T any](p *parser, n *yaml.Node, what string, item func(*yaml.Node) (T, string), dupMsg string) []T {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		p.fail(n, "%s must be a non-empty list", what)
		return nil
	}
	var out []T
	names := map[string]bool{}
	for _, en := range n.Content {
		v, name := item(en)
		if p.err != nil {
			return nil
		}
		if names[name] {
			p.fail(en, dupMsg, name)
			return nil
		}
		names[name] = true
		out = append(out, v)
	}
	return out
}
