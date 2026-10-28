package environment

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScrub(test *testing.T) {
	for _, testCase := range []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"nothing to remove", []string{"PATH=/bin", "HOME=/h"}, []string{"PATH=/bin", "HOME=/h"}},
		{
			"all four removed, order kept",
			[]string{"A=1", "GH_TOKEN=x", "GITHUB_TOKEN=y", "B=2", "GH_ENTERPRISE_TOKEN=z", "GITHUB_ENTERPRISE_TOKEN=w", "C=3"},
			[]string{"A=1", "B=2", "C=3"},
		},
		{"empty value removed", []string{"GH_TOKEN=", "A=1"}, []string{"A=1"}},
		{"similar names kept", []string{"GH_TOKEN_FILE=f", "MY_GH_TOKEN=x", "GH_HOST=h"}, []string{"GH_TOKEN_FILE=f", "MY_GH_TOKEN=x", "GH_HOST=h"}},
		{"value mentioning a token name kept", []string{"A=GH_TOKEN=x"}, []string{"A=GH_TOKEN=x"}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, Scrub(testCase.in)); diff != "" {
				test.Errorf("Scrub mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
