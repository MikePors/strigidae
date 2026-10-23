package config

import (
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// MaxFileSize is the largest config file strig will read.
const MaxFileSize = 1 << 20

// topLevel maps each top-level key to the function that decodes it. Another
// feature adding a key (a theme, merge defaults) adds an entry here and a
// field on Config.
var topLevel = map[string]func(p *parser, n *yaml.Node, c *Config){
	"tabs": (*parser).tabs,
}

var (
	login       = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\[bot\])?$`)
	placeholder = regexp.MustCompile(`\{[^}]*\}`)
)

// Parse validates data, read from the file named path (used only in errors).
// Empty data yields Default.
func Parse(path string, data []byte) (Config, error) {
	if len(data) > MaxFileSize {
		return Config{}, &Error{Path: path, Message: fmt.Sprintf("config file is too large (limit %d bytes)", MaxFileSize)}
	}
	doc, err := decodeSingle(path, data)
	if err != nil {
		return Config{}, err
	}
	if doc == nil {
		return Default(), nil
	}

	p := &parser{path: path}
	p.rejectAliases(doc)
	var c Config
	if p.err == nil {
		p.mapping(doc.Content[0], "the config file", func(key string, v *yaml.Node) {
			topLevel[key](p, v, &c)
		}, keys(topLevel))
	}
	if p.err != nil {
		return Config{}, p.err
	}
	if c.Tabs == nil {
		c.Tabs = Default().Tabs
	}
	return c, nil
}

func (p *parser) tabs(n *yaml.Node, c *Config) {
	c.Tabs = uniqueList(p, n, "tabs", p.tab, "duplicate tab name %q")
}

func (p *parser) tab(n *yaml.Node) (Tab, string) {
	t := Tab{User: Me}
	var sections *yaml.Node
	p.mapping(n, "a tab", func(key string, v *yaml.Node) {
		switch key {
		case "name":
			t.Name = p.str(v, "tab name")
		case "user":
			t.User = p.str(v, "user")
			if p.err == nil && t.User != Me && !login.MatchString(t.User) {
				p.fail(v, "user %q is not a GitHub login (letters, digits and hyphens) or %q", t.User, Me)
			}
		case "sections":
			sections = v
		}
	}, []string{"name", "user", "sections"})
	if p.err == nil && t.Name == "" {
		p.fail(n, "tab is missing a name")
	}
	if p.err != nil {
		return t, t.Name
	}
	if sections == nil {
		t.Sections = defaultSections()
	} else {
		t.Sections = uniqueList(p, sections, "sections", p.section, "duplicate section name %q in this tab")
	}
	t.resolve()
	return t, t.Name
}

func (p *parser) section(n *yaml.Node) (Section, string) {
	var s Section
	p.mapping(n, "a section", func(key string, v *yaml.Node) {
		switch key {
		case "name":
			s.Name = p.str(v, "section name")
		case "query":
			q := p.str(v, "query")
			if p.err != nil {
				return
			}
			norm, err := normalizeQuery(q)
			if err != nil {
				p.fail(v, "%s", err)
				return
			}
			s.Query = norm
		case "limit":
			s.Limit = p.int(v, "limit", 1, MaxLimit)
		case "sort":
			s.Sort = Sort(p.oneOf(v, "sort", sortNames))
		case "order":
			s.Order = Order(p.oneOf(v, "order", orderNames))
		}
	}, []string{"name", "query", "limit", "sort", "order"})
	if p.err == nil && s.Name == "" {
		p.fail(n, "section is missing a name")
	}
	if p.err == nil && s.Query == "" {
		p.fail(n, "section %q is missing a query", s.Name)
	}
	return s, s.Name
}
