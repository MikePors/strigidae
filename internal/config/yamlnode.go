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
	"time"

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

func (failure *Error) Error() string {
	if failure.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", failure.Path, failure.Line, failure.Message)
	}
	return failure.Path + ": " + failure.Message
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// decodeSingle parses data as exactly one YAML document and returns its root
// node, or nil if the file has no content.
func decodeSingle(path string, data []byte) (*yaml.Node, *Error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, syntaxError(path, err)
	}
	var second yaml.Node
	switch err := decoder.Decode(&second); {
	case err == nil:
		return nil, &Error{Path: path, Line: second.Line, Message: "the config file must contain a single YAML document"}
	case !errors.Is(err, io.EOF):
		return nil, syntaxError(path, err)
	}
	if len(document.Content) == 0 {
		return nil, nil
	}
	return &document, nil
}

// syntaxError converts a yaml.v3 error. The library reports the line of the
// construct it was parsing, which for block-structure mistakes can be the line
// where the enclosing block starts rather than the offending line.
func syntaxError(path string, err error) *Error {
	message := strings.TrimPrefix(err.Error(), "yaml: ")
	line := 0
	if messages := yamlLine.FindStringSubmatch(message); messages != nil {
		line, _ = strconv.Atoi(messages[1])
		message = strings.Replace(message, messages[0]+": ", "", 1)
	}
	return &Error{Path: path, Line: line, Message: "invalid YAML: " + message}
}

// parser records the first error; callers check err once at the end.
type parser struct {
	path string
	err  *Error
}

func (parser *parser) fail(node *yaml.Node, format string, arguments ...any) {
	if parser.err == nil {
		parser.err = &Error{Path: parser.path, Line: node.Line, Message: fmt.Sprintf(format, arguments...)}
	}
}

// rejectAliases fails on anchors, aliases and merge keys anywhere in node.
func (parser *parser) rejectAliases(node *yaml.Node) {
	if parser.err != nil {
		return
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" || node.Tag == "!!merge" {
		parser.fail(node, "YAML aliases and merge keys are not supported")
		return
	}
	for _, node := range node.Content {
		parser.rejectAliases(node)
	}
}

func keys[Value any](entries map[string]Value) []string {
	keyNames := make([]string, 0, len(entries))
	for key := range entries {
		keyNames = append(keyNames, key)
	}
	slices.Sort(keyNames)
	return keyNames
}

// mapping walks the key/value pairs of a mapping node, rejecting unknown and
// duplicate keys. what names the mapping in errors.
func (parser *parser) mapping(node *yaml.Node, what string, each func(key string, node *yaml.Node), allowed []string) {
	if node.Kind != yaml.MappingNode {
		parser.fail(node, "%s must be a mapping of keys to values", what)
		return
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, node := node.Content[i], node.Content[i+1]
		switch {
		case !slices.Contains(allowed, keyNode.Value):
			parser.fail(keyNode, "unknown key %q in %s (valid keys: %s)", keyNode.Value, what, strings.Join(allowed, ", "))
		case seen[keyNode.Value]:
			parser.fail(keyNode, "duplicate key %q in %s", keyNode.Value, what)
		}
		if parser.err != nil {
			return
		}
		seen[keyNode.Value] = true
		each(keyNode.Value, node)
		if parser.err != nil {
			return
		}
	}
}

// stringValue reads a non-empty scalar as trimmed text; numbers and booleans are
// accepted as written.
func (parser *parser) stringValue(node *yaml.Node, what string) string {
	value := strings.TrimSpace(node.Value)
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" || value == "" {
		parser.fail(node, "%s must be a non-empty text value", what)
		return ""
	}
	return value
}

// boolean reads an unquoted true or false.
func (parser *parser) boolean(node *yaml.Node, what string) bool {
	value, err := strconv.ParseBool(node.Value)
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" || err != nil {
		parser.fail(node, "%s must be true or false", what)
		return false
	}
	return value
}

// duration reads a positive Go duration such as 10s or 1m30s.
func (parser *parser) duration(node *yaml.Node, what string) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(node.Value))
	if node.Kind != yaml.ScalarNode || err != nil || value <= 0 {
		parser.fail(node, "%s must be a positive duration such as 10s or 1m30s", what)
		return 0
	}
	return value
}

// integer reads an unquoted integer from low to high.
func (parser *parser) integer(node *yaml.Node, what string, low, high int) int {
	value, err := strconv.Atoi(node.Value)
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" || err != nil || value < low || value > high {
		parser.fail(node, "%s must be a whole number from %d to %d", what, low, high)
		return 0
	}
	return value
}

// oneOf reads a text value that must be one of allowed.
func (parser *parser) oneOf(node *yaml.Node, what string, allowed []string) string {
	value := parser.stringValue(node, what)
	if parser.err == nil && !slices.Contains(allowed, value) {
		parser.fail(node, "%s %q is not one of: %s", what, value, strings.Join(allowed, ", "))
	}
	return value
}

// uniqueList reads a non-empty sequence, converting each element with item,
// which returns the value and its name. Names must be unique; duplicateMessage is the
// format (one %q for the name) of the error for a repeat. what names the list
// in the empty-list error. It returns nil if anything failed.
func uniqueList[Value any](parser *parser, node *yaml.Node, what string, item func(*yaml.Node) (Value, string), duplicateMessage string) []Value {
	if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
		parser.fail(node, "%s must be a non-empty list", what)
		return nil
	}
	var out []Value
	names := map[string]bool{}
	for _, entryNode := range node.Content {
		value, name := item(entryNode)
		if parser.err != nil {
			return nil
		}
		if names[name] {
			parser.fail(entryNode, duplicateMessage, name)
			return nil
		}
		names[name] = true
		out = append(out, value)
	}
	return out
}
