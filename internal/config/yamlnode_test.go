package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v3"
)

// node parses src and returns its root node.
func node(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func errText(p *parser) string {
	if p.err == nil {
		return ""
	}
	return p.err.Error()
}

func TestParserInt(t *testing.T) {
	tests := []struct {
		name    string
		src     string
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &parser{path: "c.yml"}
			got := p.int(node(t, tt.src), "n", 1, 100)
			if got != tt.want {
				t.Errorf("value = %d, want %d", got, tt.want)
			}
			if diff := cmp.Diff(tt.wantErr, errText(p)); diff != "" {
				t.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParserOneOf(t *testing.T) {
	allowed := []string{"asc", "desc"}
	tests := []struct {
		name    string
		src     string
		want    string
		wantErr string
	}{
		{"valid", "asc", "asc", ""},
		{"invalid", "up", "up", `c.yml:1: order "up" is not one of: asc, desc`},
		{"empty", "''", "", "c.yml:1: order must be a non-empty text value"},
		{"not a scalar", "[asc]", "", "c.yml:1: order must be a non-empty text value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &parser{path: "c.yml"}
			got := p.oneOf(node(t, tt.src), "order", allowed)
			if got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
			if diff := cmp.Diff(tt.wantErr, errText(p)); diff != "" {
				t.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUniqueList(t *testing.T) {
	read := func(p *parser) func(*yaml.Node) (string, string) {
		return func(n *yaml.Node) (string, string) {
			s := p.str(n, "item")
			return "<" + s + ">", s
		}
	}
	tests := []struct {
		name    string
		src     string
		want    []string
		wantErr string
	}{
		{"items in order", "[a, b]", []string{"<a>", "<b>"}, ""},
		{"empty list", "[]", nil, "c.yml:1: things must be a non-empty list"},
		{"not a list", "a", nil, "c.yml:1: things must be a non-empty list"},
		{"duplicate", "- a\n- b\n- a\n", nil, `c.yml:3: duplicate thing "a"`},
		{"item error stops", "- a\n- ''\n- b\n", nil, "c.yml:2: item must be a non-empty text value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &parser{path: "c.yml"}
			got := uniqueList(p, node(t, tt.src), "things", read(p), "duplicate thing %q")
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("items (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantErr, errText(p)); diff != "" {
				t.Errorf("error (-want +got):\n%s", diff)
			}
		})
	}
}
