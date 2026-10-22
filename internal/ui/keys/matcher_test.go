package keys

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/google/go-cmp/cmp"
)

func bind(keys ...string) key.Binding { return key.NewBinding(key.WithKeys(keys...)) }

func TestMatcherFeed(t *testing.T) {
	disabled := bind("x y")
	disabled.SetEnabled(false)

	tests := []struct {
		name     string
		bindings []key.Binding
		keys     []string
		want     []Result
	}{
		{
			name:     "single key",
			bindings: []key.Binding{bind("j")},
			keys:     []string{"j"},
			want:     []Result{{Matched, "j"}},
		},
		{
			name:     "unbound key",
			bindings: []key.Binding{bind("j")},
			keys:     []string{"z"},
			want:     []Result{{NoMatch, "z"}},
		},
		{
			name:     "sequence completes",
			bindings: []key.Binding{bind("g g")},
			keys:     []string{"g", "g"},
			want:     []Result{{Pending, "g"}, {Matched, "g g"}},
		},
		{
			name:     "two sequences",
			bindings: []key.Binding{bind("g t"), bind("z c")},
			keys:     []string{"g", "t", "z", "c"},
			want:     []Result{{Pending, "g"}, {Matched, "g t"}, {Pending, "z"}, {Matched, "z c"}},
		},
		{
			name:     "navigation brackets are single keys",
			bindings: []key.Binding{bind("]"), bind("["), bind("}"), bind("{"), bind(")"), bind("("), bind("g g")},
			keys:     []string{"]", "[", "}", "{", ")", "(", "]"},
			want: []Result{
				{Matched, "]"},
				{Matched, "["},
				{Matched, "}"},
				{Matched, "{"},
				{Matched, ")"},
				{Matched, "("},
				{Matched, "]"},
			},
		},
		{
			name:     "broken sequence falls back to key alone",
			bindings: []key.Binding{bind("g g"), bind("j")},
			keys:     []string{"g", "j"},
			want:     []Result{{Pending, "g"}, {Matched, "j"}},
		},
		{
			name:     "broken sequence with unbound key",
			bindings: []key.Binding{bind("g g")},
			keys:     []string{"g", "x"},
			want:     []Result{{Pending, "g"}, {NoMatch, "x"}},
		},
		{
			name:     "ambiguous prefix stays pending",
			bindings: []key.Binding{bind("g"), bind("g g")},
			keys:     []string{"g"},
			want:     []Result{{Pending, "g"}},
		},
		{
			name:     "ambiguous prefix completes longer",
			bindings: []key.Binding{bind("g"), bind("g g")},
			keys:     []string{"g", "g"},
			want:     []Result{{Pending, "g"}, {Matched, "g g"}},
		},
		{
			name:     "three keys",
			bindings: []key.Binding{bind("z a b")},
			keys:     []string{"z", "a", "b"},
			want:     []Result{{Pending, "z"}, {Pending, "z a"}, {Matched, "z a b"}},
		},
		{
			name:     "disabled binding ignored",
			bindings: []key.Binding{disabled},
			keys:     []string{"x"},
			want:     []Result{{NoMatch, "x"}},
		},
		{
			name:     "alternative keys",
			bindings: []key.Binding{bind("j", "down")},
			keys:     []string{"down"},
			want:     []Result{{Matched, "down"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Matcher
			var got []Result
			for _, k := range tt.keys {
				got = append(got, m.Feed(k, tt.bindings))
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("results mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatcherExpire(t *testing.T) {
	tests := []struct {
		name     string
		bindings []key.Binding
		keys     []string
		want     Result
	}{
		{"nothing pending", []key.Binding{bind("g g")}, nil, Result{Kind: NoMatch}},
		{"incomplete prefix is dropped", []key.Binding{bind("g g")}, []string{"g"}, Result{NoMatch, "g"}},
		{"ambiguous prefix resolves to shorter", []key.Binding{bind("g"), bind("g g")}, []string{"g"}, Result{Matched, "g"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Matcher
			for _, k := range tt.keys {
				m.Feed(k, tt.bindings)
			}
			if got := m.Expire(tt.bindings); got != tt.want {
				t.Errorf("Expire() = %+v, want %+v", got, tt.want)
			}
			if m.Pending() {
				t.Error("still pending after Expire")
			}
		})
	}
}

func TestMatcherDisplayAndReset(t *testing.T) {
	var m Matcher
	bs := []key.Binding{bind("z a b")}
	m.Feed("z", bs)
	m.Feed("a", bs)
	if got := m.Display(); got != "za" {
		t.Errorf("Display() = %q, want %q", got, "za")
	}
	m.Reset()
	if m.Pending() || m.Display() != "" {
		t.Error("Reset did not clear pending keys")
	}
}

func TestGlobalBindings(t *testing.T) {
	g := DefaultGlobal()
	if !key.Matches(Msg{Seq: "g g"}, g.Top) {
		t.Error("g g should match Top")
	}
	if !key.Matches(Msg{Seq: "R"}, g.Refresh) || key.Matches(Msg{Seq: "r"}, g.Refresh) {
		t.Error("refresh must be R, not r")
	}
	if !key.Matches(Msg{Seq: ":"}, g.Command) {
		t.Error(": should open the command line")
	}
	seen := map[string]bool{}
	for _, b := range g.Bindings() {
		for _, k := range b.Keys() {
			if seen[k] {
				t.Errorf("duplicate global key %q", k)
			}
			seen[k] = true
		}
	}
	for _, k := range []string{"r", "c", "q", "ctrl+c", "tab", "ctrl+o", "ctrl+i"} {
		if seen[k] {
			t.Errorf("%q is reserved and must not be bound globally", k)
		}
	}
}
