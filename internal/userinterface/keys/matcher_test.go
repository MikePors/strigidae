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
			want:     []Result{{Kind: Matched, Sequence: "j"}},
		},
		{
			name:     "unbound key",
			bindings: []key.Binding{bind("j")},
			keys:     []string{"z"},
			want:     []Result{{Kind: NoMatch, Sequence: "z"}},
		},
		{
			name:     "sequence completes",
			bindings: []key.Binding{bind("g g")},
			keys:     []string{"g", "g"},
			want:     []Result{{Kind: Pending, Sequence: "g"}, {Kind: Matched, Sequence: "g g"}},
		},
		{
			name:     "two sequences",
			bindings: []key.Binding{bind("g t"), bind("z c")},
			keys:     []string{"g", "t", "z", "c"},
			want:     []Result{{Kind: Pending, Sequence: "g"}, {Kind: Matched, Sequence: "g t"}, {Kind: Pending, Sequence: "z"}, {Kind: Matched, Sequence: "z c"}},
		},
		{
			name:     "navigation brackets are single keys",
			bindings: []key.Binding{bind("]"), bind("["), bind("}"), bind("{"), bind(")"), bind("("), bind("g g")},
			keys:     []string{"]", "[", "}", "{", ")", "(", "]"},
			want: []Result{
				{Kind: Matched, Sequence: "]"},
				{Kind: Matched, Sequence: "["},
				{Kind: Matched, Sequence: "}"},
				{Kind: Matched, Sequence: "{"},
				{Kind: Matched, Sequence: ")"},
				{Kind: Matched, Sequence: "("},
				{Kind: Matched, Sequence: "]"},
			},
		},
		{
			name:     "broken sequence falls back to key alone",
			bindings: []key.Binding{bind("g g"), bind("j")},
			keys:     []string{"g", "j"},
			want:     []Result{{Kind: Pending, Sequence: "g"}, {Kind: Matched, Sequence: "j"}},
		},
		{
			name:     "broken sequence with unbound key",
			bindings: []key.Binding{bind("g g")},
			keys:     []string{"g", "x"},
			want:     []Result{{Kind: Pending, Sequence: "g"}, {Kind: NoMatch, Sequence: "x"}},
		},
		{
			name:     "ambiguous prefix stays pending",
			bindings: []key.Binding{bind("g"), bind("g g")},
			keys:     []string{"g"},
			want:     []Result{{Kind: Pending, Sequence: "g"}},
		},
		{
			name:     "ambiguous prefix completes longer",
			bindings: []key.Binding{bind("g"), bind("g g")},
			keys:     []string{"g", "g"},
			want:     []Result{{Kind: Pending, Sequence: "g"}, {Kind: Matched, Sequence: "g g"}},
		},
		{
			name:     "three keys",
			bindings: []key.Binding{bind("z a b")},
			keys:     []string{"z", "a", "b"},
			want:     []Result{{Kind: Pending, Sequence: "z"}, {Kind: Pending, Sequence: "z a"}, {Kind: Matched, Sequence: "z a b"}},
		},
		{
			name:     "disabled binding ignored",
			bindings: []key.Binding{disabled},
			keys:     []string{"x"},
			want:     []Result{{Kind: NoMatch, Sequence: "x"}},
		},
		{
			name:     "alternative keys",
			bindings: []key.Binding{bind("j", "down")},
			keys:     []string{"down"},
			want:     []Result{{Kind: Matched, Sequence: "down"}},
		},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var matcher Matcher
			var got []Result
			for _, pressed := range testCase.keys {
				got = append(got, matcher.Feed(pressed, testCase.bindings))
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("results mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatcherCounts(test *testing.T) {
	bindings := []key.Binding{bind("j"), bind("G"), bind("z z"), bind("esc")}
	count := func(number int, sequence string) Result {
		return Result{Kind: Matched, Sequence: sequence, Count: number}
	}
	typing := Result{Kind: Pending}
	tests := []struct {
		name string
		keys []string
		want []Result
	}{
		{"single digit", []string{"5", "j"}, []Result{typing, count(5, "j")}},
		{"multiple digits", []string{"1", "1", "j"}, []Result{typing, typing, count(11, "j")}},
		{"zero continues a count", []string{"1", "0", "j"}, []Result{typing, typing, count(10, "j")}},
		{"leading zero is not a count", []string{"0", "j"}, []Result{{Kind: NoMatch, Sequence: "0"}, count(0, "j")}},
		{"jump to line", []string{"4", "2", "G"}, []Result{typing, typing, count(42, "G")}},
		{"count is capped", []string{"9", "9", "9", "9", "9", "9", "j"}, []Result{typing, typing, typing, typing, typing, typing, count(MaximumCount, "j")}},
		{"unbound key clears the count", []string{"3", "x", "j"}, []Result{typing, {Kind: NoMatch, Sequence: "x"}, count(0, "j")}},
		{"esc clears the count", []string{"3", "esc", "j"}, []Result{typing, count(3, "esc"), count(0, "j")}},
		{"count is kept through a sequence", []string{"3", "z", "z"}, []Result{typing, {Kind: Pending, Sequence: "z"}, count(3, "z z")}},
		{"count survives a broken sequence", []string{"3", "z", "j"}, []Result{typing, {Kind: Pending, Sequence: "z"}, count(3, "j")}},
		{"count does not join across a broken prefix", []string{"3", "z", "5", "j"}, []Result{typing, {Kind: Pending, Sequence: "z"}, typing, count(5, "j")}},
		{"digit after a prefix restarts a count", []string{"z", "5", "j"}, []Result{{Kind: Pending, Sequence: "z"}, typing, count(5, "j")}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var matcher Matcher
			var got []Result
			for _, pressed := range testCase.keys {
				got = append(got, matcher.Feed(pressed, bindings))
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				test.Errorf("results mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMatcherCountDisplayAndExpire(test *testing.T) {
	bindings := []key.Binding{bind("j"), bind("g g")}
	var matcher Matcher
	matcher.Feed("1", bindings)
	matcher.Feed("1", bindings)
	if !matcher.Pending() || matcher.Display() != "11" {
		test.Errorf("Pending() = %v, Display() = %q; want a pending %q", matcher.Pending(), matcher.Display(), "11")
	}
	matcher.Feed("g", bindings)
	if got := matcher.Display(); got != "11g" {
		test.Errorf("Display() = %q, want %q", got, "11g")
	}
	matcher.Reset()
	matcher.Feed("7", bindings)
	if got := matcher.Expire(bindings); got.Kind != NoMatch {
		test.Errorf("Expire() = %+v, want NoMatch", got)
	}
	if matcher.Pending() || matcher.Feed("j", bindings).Count != 0 {
		test.Error("the count survived the timeout")
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
		{"incomplete prefix is dropped", []key.Binding{bind("g g")}, []string{"g"}, Result{Kind: NoMatch, Sequence: "g"}},
		{"ambiguous prefix resolves to shorter", []key.Binding{bind("g"), bind("g g")}, []string{"g"}, Result{Kind: Matched, Sequence: "g"}},
		{"count is delivered with the shorter", []key.Binding{bind("g"), bind("g g")}, []string{"4", "g"}, Result{Kind: Matched, Sequence: "g", Count: 4}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			var matcher Matcher
			for _, pressed := range testCase.keys {
				matcher.Feed(pressed, testCase.bindings)
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
		for _, pressed := range binding.Keys() {
			if seen[pressed] {
				test.Errorf("duplicate global key %q", pressed)
			}
			seen[pressed] = true
		}
	}
	for _, pressed := range []string{"r", "c", "q", "ctrl+c", "tab", "ctrl+o", "ctrl+i"} {
		if seen[pressed] {
			test.Errorf("%q is reserved and must not be bound globally", pressed)
		}
	}
}
