package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// KeyOverride is one line of the keys: section: the keys an action should have
// instead of its defaults. The config package does not know the action names
// (they belong to the user interface), so the program checks them and the
// keys, and reports a problem at Line.
type KeyOverride struct {
	Action string
	// Keys are the keys as written: each a key or a space-separated sequence.
	Keys []string
	// Line is the line of the action name in the file.
	Line int
}

// keys reads the mapping of action names to one key or a list of keys. Any
// name is accepted here; see KeyOverride.
func (parser *parser) keys(node *yaml.Node, config *Config) {
	if node.Kind != yaml.MappingNode || len(node.Content) == 0 {
		parser.fail(node, "keys must be a mapping of action names to keys")
		return
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		nameNode, valueNode := node.Content[i], node.Content[i+1]
		if nameNode.Kind != yaml.ScalarNode {
			parser.fail(nameNode, "an action name in keys must be plain text")
			return
		}
		if seen[nameNode.Value] {
			parser.fail(nameNode, "duplicate action %q in keys", nameNode.Value)
			return
		}
		seen[nameNode.Value] = true
		override := KeyOverride{Action: nameNode.Value, Line: nameNode.Line}
		override.Keys = parser.keyList(valueNode, nameNode.Value)
		if parser.err != nil {
			return
		}
		config.Keys = append(config.Keys, override)
	}
}

// keyList reads one key or a non-empty list of keys.
func (parser *parser) keyList(node *yaml.Node, action string) []string {
	what := fmt.Sprintf("a key of %q", action)
	if node.Kind != yaml.SequenceNode {
		return []string{parser.stringValue(node, what)}
	}
	if len(node.Content) == 0 {
		parser.fail(node, "%q needs at least one key", action)
		return nil
	}
	var pressed []string
	for _, entry := range node.Content {
		pressed = append(pressed, parser.stringValue(entry, what))
		if parser.err != nil {
			return nil
		}
	}
	return pressed
}
