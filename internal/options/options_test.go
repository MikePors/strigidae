package options

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDefaultsAndWith(test *testing.T) {
	var values Values
	if !values.WrapScan() || !values.HighlightSearch() {
		test.Fatal("the zero value should be every option at its default (on)")
	}
	changed := values.With(WrapScan, false)
	if changed.WrapScan() || !changed.HighlightSearch() {
		test.Errorf("With(wrapscan, false) = %v", changed)
	}
	if !values.WrapScan() {
		test.Error("With changed the receiver")
	}
}

func TestString(test *testing.T) {
	for name, testCase := range map[string]struct {
		values Values
		want   string
	}{
		"defaults": {nil, "wrapscan hlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber"},
		"mixed":    {Values{WrapScan: false}, "nowrapscan hlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber"},
		"all off":  {Values{WrapScan: false, HighlightSearch: false}, "nowrapscan nohlsearch nowrap nohideviewed nohidereviewed noautoviewed nohideresolved noskipresolved norelativenumber"},
	} {
		test.Run(name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, testCase.values.String()); diff != "" {
				test.Error(diff)
			}
		})
	}
}

func TestDiffers(test *testing.T) {
	for name, testCase := range map[string]struct {
		values Values
		option string
		want   bool
	}{
		"unset":                  {nil, WrapScan, false},
		"set to the default":     {Values{WrapScan: true}, WrapScan, false},
		"off by default, set on": {Values{Wrap: true}, Wrap, true},
		"on by default, set off": {Values{WrapScan: false}, WrapScan, true},
		"another option differs": {Values{Wrap: true}, WrapScan, false},
	} {
		test.Run(name, func(test *testing.T) {
			if got := testCase.values.Differs(testCase.option); got != testCase.want {
				test.Errorf("Differs(%q) = %v, want %v", testCase.option, got, testCase.want)
			}
		})
	}
}

func TestChanged(test *testing.T) {
	for name, testCase := range map[string]struct {
		values Values
		want   string
	}{
		"defaults":             {nil, ""},
		"set to the default":   {Values{WrapScan: true, Wrap: false}, ""},
		"one off":              {Values{WrapScan: false}, "nowrapscan"},
		"one on":               {Values{Wrap: true}, "wrap"},
		"in declaration order": {Values{RelativeNumber: true, HighlightSearch: false, WrapScan: true}, "nohlsearch relativenumber"},
	} {
		test.Run(name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, testCase.values.Changed()); diff != "" {
				test.Error(diff)
			}
		})
	}
}

func TestDeclarationsAreUnique(test *testing.T) {
	seen := map[string]bool{}
	for _, option := range All() {
		// :set all lists every option, so no option may take that name.
		if option.Name == "" || option.Name == "all" || seen[option.Name] {
			test.Errorf("bad or duplicate declaration: %+v", option)
		}
		seen[option.Name] = true
	}
}

func TestEveryOptionHasADescription(test *testing.T) {
	for _, option := range All() {
		if strings.TrimSpace(option.Description) == "" {
			test.Errorf("option %q has no description", option.Name)
		}
	}
}
