// Package options declares the yes/no settings that both the config file and
// the `:set` command can change. The list is written once here, so the two
// can't drift apart. It is a non-UI package: the config parser and the user
// interface both read it.
package options

import "strings"

// Names of the options.
const (
	WrapScan        = "wrapscan"
	HighlightSearch = "hlsearch"
)

// Option is one declared setting.
type Option struct {
	// Name is the config key and the `:set` name.
	Name string
	// Default is the value when neither the config nor `:set` changed it.
	Default bool
}

// All returns every option, in the order `:set` lists them.
func All() []Option {
	return []Option{
		{Name: WrapScan, Default: true},
		{Name: HighlightSearch, Default: true},
	}
}

// Lookup returns the option called name.
func Lookup(name string) (Option, bool) {
	for _, option := range All() {
		if option.Name == name {
			return option, true
		}
	}
	return Option{}, false
}

// Values holds the settings that differ from the defaults. The zero value (a
// nil map) is every option at its default.
type Values map[string]bool

// Get returns the value of the option called name; an undeclared name is off.
func (values Values) Get(name string) bool {
	if value, ok := values[name]; ok {
		return value
	}
	option, _ := Lookup(name)
	return option.Default
}

// With returns a copy of values with the option called name set to value.
func (values Values) With(name string, value bool) Values {
	updated := make(Values, len(values)+1)
	for key, current := range values {
		updated[key] = current
	}
	updated[name] = value
	return updated
}

// WrapScan reports whether searches wrap around the ends of a list.
func (values Values) WrapScan() bool { return values.Get(WrapScan) }

// HighlightSearch reports whether rows matching a search are marked.
func (values Values) HighlightSearch() bool { return values.Get(HighlightSearch) }

// Describe writes one option's value the way `:set` shows it: its name, or
// "no" and its name.
func (values Values) Describe(name string) string {
	if values.Get(name) {
		return name
	}
	return "no" + name
}

// String describes every option, for example "wrapscan nohlsearch".
func (values Values) String() string {
	var words []string
	for _, option := range All() {
		words = append(words, values.Describe(option.Name))
	}
	return strings.Join(words, " ")
}
