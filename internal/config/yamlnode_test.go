package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v3"
)

// node parses src and returns its root node.
func node(test *testing.T, source string) *yaml.Node {
	test.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		test.Fatal(err)
	}
	return document.Content[0]
}

func errText(parser *parser) string {
	if parser.err == nil {
		return ""
	}
	return parser.err.Error()
}

func TestParserInteger(test *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    int
		wantErr string
	}{
		{"in range", "5", 5, ""},
		{"minimum", "1", 1, ""},
		{"maximum", "100", 100, ""},
		{"too small", "0", 0, "c.yml:1: n must be a whole number from 1 to 100"},
		{"too big", "101", 0, "c.yml:1: n must be a whole number from 1 to 100"},
		{"quoted", "'5'", 0, "c.yml:1: n must be a whole number from 1 to 100"},
		{"float", "5.5", 0, "c.yml:1: n must be a whole number from 1 to 100"},
		{"text", "many", 0, "c.yml:1: n must be a whole number from 1 to 100"},
		{"list", "[1]", 0, "c.yml:1: n must be a whole number from 1 to 100"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			nodeParser := &parser{path: "c.yml"}
			got := nodeParser.integer(node(test, testCase.source), "n", 1, 100)
			if got != testCase.want {
				test.Errorf("value = %d, want %d", got, testCase.want)
			}
			if diff := cmp.Diff(testCase.wantErr, errText(nodeParser)); diff != "" {
				test.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParserOneOf(test *testing.T) {
	allowed := []string{"asc", "desc"}
	tests := []struct {
		name    string
		source  string
		want    string
		wantErr string
	}{
		{"valid", "asc", "asc", ""},
		{"invalid", "up", "up", `c.yml:1: order "up" is not one of: asc, desc`},
		{"empty", "''", "", "c.yml:1: order must be a non-empty text value"},
		{"not a scalar", "[asc]", "", "c.yml:1: order must be a non-empty text value"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			nodeParser := &parser{path: "c.yml"}
			got := nodeParser.oneOf(node(test, testCase.source), "order", allowed)
			if got != testCase.want {
				test.Errorf("value = %q, want %q", got, testCase.want)
			}
			if diff := cmp.Diff(testCase.wantErr, errText(nodeParser)); diff != "" {
				test.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUniqueList(test *testing.T) {
	read := func(parser *parser) func(*yaml.Node) (string, string) {
		return func(node *yaml.Node) (string, string) {
			text := parser.stringValue(node, "item")
			return "<" + text + ">", text
		}
	}
	tests := []struct {
		name    string
		source  string
		want    []string
		wantErr string
	}{
		{"items in order", "[a, b]", []string{"<a>", "<b>"}, ""},
		{"empty list", "[]", nil, "c.yml:1: things must be a non-empty list"},
		{"not a list", "a", nil, "c.yml:1: things must be a non-empty list"},
		{"duplicate", "- a\n- b\n- a\n", nil, `c.yml:3: duplicate thing "a"`},
		{"item error stops", "- a\n- ''\n- b\n", nil, "c.yml:2: item must be a non-empty text value"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			nodeParser := &parser{path: "c.yml"}
			got := uniqueList(nodeParser, node(test, testCase.source), "things", read(nodeParser), "duplicate thing %q")
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("items (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(testCase.wantErr, errText(nodeParser)); diff != "" {
				test.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}
