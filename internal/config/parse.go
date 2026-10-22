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

// Error is a problem in the config file, located by line.
type Error struct {
	Path string
	Line int // 1-based; 0 if unknown
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Msg)
	}
	return e.Path + ": " + e.Msg
}

// topLevel maps each top-level key to the function that decodes it. Another
// feature adding a key (a theme, merge defaults) adds an entry here and a
// field on Config.
var topLevel = map[string]func(p *parser, n *yaml.Node, c *Config){
	"tabs": (*parser).tabs,
}

var (
	yamlLine    = regexp.MustCompile(`line (\d+)`)
	login       = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\[bot\])?$`)
	placeholder = regexp.MustCompile(`\{[^}]*\}`)
)

// Parse validates data, read from the file named path (used only in errors).
// Empty data yields Default.
func Parse(path string, data []byte) (Config, error) {
	if len(data) > MaxFileSize {
		return Config{}, &Error{Path: path, Msg: fmt.Sprintf("config file is too large (limit %d bytes)", MaxFileSize)}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return Default(), nil
		}
		return Config{}, syntaxError(path, err)
	}
	var second yaml.Node
	switch err := dec.Decode(&second); {
	case err == nil:
		return Config{}, &Error{Path: path, Line: second.Line, Msg: "the config file must contain a single YAML document"}
	case !errors.Is(err, io.EOF):
		return Config{}, syntaxError(path, err)
	}
	if len(doc.Content) == 0 {
		return Default(), nil
	}

	p := &parser{path: path}
	p.rejectAliases(&doc)
	if p.err != nil {
		return Config{}, p.err
	}
	var c Config
	p.mapping(doc.Content[0], "the config file", func(key string, v *yaml.Node) {
		topLevel[key](p, v, &c)
	}, keys(topLevel))
	if p.err != nil {
		return Config{}, p.err
	}
	if c.Tabs == nil {
		c.Tabs = Default().Tabs
	}
	return c, nil
}

// MaxFileSize is the largest config file strig will read.
const MaxFileSize = 1 << 20

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
	return &Error{Path: path, Line: line, Msg: "invalid YAML: " + msg}
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

// parser records the first error; callers check err once at the end.
type parser struct {
	path string
	err  *Error
}

func (p *parser) fail(n *yaml.Node, format string, args ...any) {
	if p.err == nil {
		p.err = &Error{Path: p.path, Line: n.Line, Msg: fmt.Sprintf(format, args...)}
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

func (p *parser) str(n *yaml.Node, what string) string {
	v := strings.TrimSpace(n.Value)
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" || v == "" {
		p.fail(n, "%s must be a non-empty text value", what)
		return ""
	}
	return v
}

func (p *parser) tabs(n *yaml.Node, c *Config) {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		p.fail(n, "tabs must be a non-empty list")
		return
	}
	names := map[string]bool{}
	for _, tn := range n.Content {
		t := p.tab(tn)
		if p.err != nil {
			return
		}
		if names[t.Name] {
			p.fail(tn, "duplicate tab name %q", t.Name)
			return
		}
		names[t.Name] = true
		c.Tabs = append(c.Tabs, t)
	}
}

func (p *parser) tab(n *yaml.Node) Tab {
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
	if p.err != nil {
		return t
	}
	if t.Name == "" {
		p.fail(n, "tab is missing a name")
		return t
	}
	if sections == nil {
		t.Sections = defaultSections()
	} else {
		t.Sections = p.sections(sections)
	}
	t.resolve()
	return t
}

func (p *parser) sections(n *yaml.Node) []Section {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		p.fail(n, "sections must be a non-empty list")
		return nil
	}
	var out []Section
	names := map[string]bool{}
	for _, sn := range n.Content {
		s := p.section(sn)
		if p.err != nil {
			return nil
		}
		if names[s.Name] {
			p.fail(sn, "duplicate section name %q in this tab", s.Name)
			return nil
		}
		names[s.Name] = true
		out = append(out, s)
	}
	return out
}

func (p *parser) section(n *yaml.Node) Section {
	var s Section
	p.mapping(n, "a section", func(key string, v *yaml.Node) {
		switch key {
		case "name":
			s.Name = p.str(v, "section name")
		case "query":
			s.Query = p.str(v, "query")
			if p.err != nil {
				return
			}
			hasPR := false
			for _, tok := range strings.Fields(strings.ToLower(s.Query)) {
				switch {
				case strings.HasPrefix(tok, "sort:"):
					p.fail(v, "query must not contain sort:; use the sort and order keys")
					return
				case tok == "is:pr" || tok == "type:pr":
					hasPR = true
				}
			}
			if !hasPR {
				s.Query = "is:pr " + s.Query
			}
			for _, ph := range placeholder.FindAllString(s.Query, -1) {
				if ph != "{user}" {
					p.fail(v, "unknown variable %s in query (the only variable is {user})", ph)
					return
				}
			}
		case "limit":
			lim, err := strconv.Atoi(v.Value)
			if v.Kind != yaml.ScalarNode || v.Tag != "!!int" || err != nil || lim < 1 || lim > MaxLimit {
				p.fail(v, "limit must be a whole number from 1 to %d", MaxLimit)
				return
			}
			s.Limit = lim
		case "sort":
			s.Sort = Sort(p.str(v, "sort"))
			if p.err == nil && !slices.Contains(sorts, s.Sort) {
				p.fail(v, "sort %q is not one of: %s", s.Sort, joinSorts())
			}
		case "order":
			s.Order = Order(p.str(v, "order"))
			if p.err == nil && s.Order != OrderAsc && s.Order != OrderDesc {
				p.fail(v, "order %q is not one of: asc, desc", s.Order)
			}
		}
	}, []string{"name", "query", "limit", "sort", "order"})
	if p.err == nil && s.Name == "" {
		p.fail(n, "section is missing a name")
	}
	if p.err == nil && s.Query == "" {
		p.fail(n, "section %q is missing a query", s.Name)
	}
	return s
}

func joinSorts() string {
	ss := make([]string, len(sorts))
	for i, s := range sorts {
		ss[i] = string(s)
	}
	return strings.Join(ss, ", ")
}
