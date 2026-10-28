package options

import (
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

func TestDeclarationsAreUnique(test *testing.T) {
	seen := map[string]bool{}
	for _, option := range All() {
		if option.Name == "" || seen[option.Name] {
			test.Errorf("bad or duplicate declaration: %+v", option)
		}
		seen[option.Name] = true
	}
}
