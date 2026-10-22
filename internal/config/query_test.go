package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"adds is:pr", "label:bug", "is:pr label:bug", ""},
		{"keeps is:pr", "is:open is:pr", "is:open is:pr", ""},
		{"is:pr case-insensitive", "IS:PR label:x", "IS:PR label:x", ""},
		{"type:pr counts", "type:pr label:x", "type:pr label:x", ""},
		{"is:pr as a substring does not count", "label:is:pro", "is:pr label:is:pro", ""},
		{"user variable allowed", "author:{user}", "is:pr author:{user}", ""},
		{"sort rejected", "is:pr sort:created-asc", "", "query must not contain sort:; use the sort and order keys"},
		{"sort case-insensitive", "is:pr SORT:updated", "", "query must not contain sort:; use the sort and order keys"},
		{"unknown variable", "author:{usr}", "", "unknown variable {usr} in query (the only variable is {user})"},
		{"sort inside a word is fine", "label:resort:x", "is:pr label:resort:x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeQuery(tt.in)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if diff := cmp.Diff(tt.wantErr, gotErr); diff != "" {
				t.Fatalf("error (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("query (-want +got):\n%s", diff)
			}
		})
	}
}
