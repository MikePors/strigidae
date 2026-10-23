package keys

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/google/go-cmp/cmp"
)

func bind(keys ...string) key.Binding { return key.NewBinding(key.WithKeys(keys...)) }

func TestMatcherFeed(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var matcher Matcher
			var got []Result
			for _, k := range testCase.keys {
				got = append(got, matcher.Feed(k, testCase.bindings))
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("results mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatcherExpire(test *testing.T) {
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
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var matcher Matcher
			for _, k := range testCase.keys {
				matcher.Feed(k, testCase.bindings)
			}
			if got := matcher.Expire(testCase.bindings); got != testCase.want {
				test.Errorf("Expire() = %+v, want %+v", got, testCase.want)
			}
			if matcher.Pending() {
				test.Error("still pending after Expire")
			}
		})
	}
}

func TestMatcherDisplayAndReset(test *testing.T) {
	var matcher Matcher
	bindings := []key.Binding{bind("z a b")}
	matcher.Feed("z", bindings)
	matcher.Feed("a", bindings)
	if got := matcher.Display(); got != "za" {
		test.Errorf("Display() = %q, want %q", got, "za")
	}
	matcher.Reset()
	if matcher.Pending() || matcher.Display() != "" {
		test.Error("Reset did not clear pending keys")
	}
}

func TestGlobalBindings(test *testing.T) {
	global := DefaultGlobal()
	if !key.Matches(Message{Sequence: "g g"}, global.Top) {
		test.Error("g g should match Top")
	}
	if !key.Matches(Message{Sequence: "R"}, global.Refresh) || key.Matches(Message{Sequence: "r"}, global.Refresh) {
		test.Error("refresh must be R, not r")
	}
	if !key.Matches(Message{Sequence: ":"}, global.Command) {
		test.Error(": should open the command line")
	}
	seen := map[string]bool{}
	for _, binding := range global.Bindings() {
		for _, k := range binding.Keys() {
			if seen[k] {
				test.Errorf("duplicate global key %q", k)
			}
			seen[k] = true
		}
	}
	for _, k := range []string{"r", "c", "q", "ctrl+c", "tab", "ctrl+o", "ctrl+i"} {
		if seen[k] {
			test.Errorf("%q is reserved and must not be bound globally", k)
		}
	}
}
