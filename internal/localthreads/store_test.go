package localthreads

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

var (
	widgets   = github.Repository{Owner: "acme", Name: "widgets"}
	widgetKey = Key{Host: "github.com", Repository: widgets, Number: 7}
	baseTime  = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
)

// asked is an entry of widgetKey's pull request.
func asked(question string, when time.Time) Entry {
	return Entry{Host: "github.com", Repository: widgets, Number: 7, Question: question, Asked: when}
}

// bigAnswer is an answer of about size bytes of ordinary text.
func bigAnswer(size int) string { return strings.Repeat("0123456789abcdef", size/16) }

func questions(entries []Entry) []string {
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Question)
	}
	return got
}

func identifiers(entries []Entry) []string {
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Identifier)
	}
	return got
}

func TestAddGivesIdentifiersInOrder(test *testing.T) {
	var store Store
	first := store.Add(asked("one", baseTime))
	second := store.Add(asked("two", baseTime))
	if first != "local-1" || second != "local-2" {
		test.Errorf("identifiers = %q, %q", first, second)
	}
}

func TestDefaultLimit(test *testing.T) {
	var store Store
	for i := 0; i < DefaultLimit+5; i++ {
		store.Add(Entry{Number: 1, Asked: time.Unix(int64(i), 0)})
	}
	if got := len(store.ForRequest(Key{Number: 1})); got != DefaultLimit {
		test.Errorf("kept %d, want %d", got, DefaultLimit)
	}
}

func TestAddDropsTheOldestByAskedTime(test *testing.T) {
	for _, testCase := range []struct {
		name  string
		limit int
		times []time.Duration // hours after baseTime, in the order asked
		want  []string
	}{
		{"the second stored is the oldest", 3, []time.Duration{2, 0, 1, 3}, []string{"q1", "q3", "q4"}},
		{"the newest stored is the oldest", 3, []time.Duration{1, 2, 3, 0}, []string{"q1", "q2", "q3"}},
		{"equal times drop the one stored first", 2, []time.Duration{1, 1, 1}, []string{"q2", "q3"}},
		{"within the limit nothing goes", 4, []time.Duration{3, 2, 1, 0}, []string{"q1", "q2", "q3", "q4"}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			store := Store{Limit: testCase.limit}
			for i, hours := range testCase.times {
				store.Add(asked("q"+string(rune('1'+i)), baseTime.Add(hours*time.Hour)))
			}
			if diff := cmp.Diff(testCase.want, questions(store.ForRequest(widgetKey))); diff != "" {
				test.Errorf("kept (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLimitIsPerPullRequest(test *testing.T) {
	store := Store{Limit: 1}
	store.Add(asked("seven", baseTime))
	other := asked("eight", baseTime)
	other.Number = 8
	store.Add(other)
	if got := len(store.Snapshot()); got != 2 {
		test.Errorf("%d threads kept, want one for each pull request", got)
	}
}

func TestFitDropsTheOldestBySize(test *testing.T) {
	entryOf := func(question string, hours time.Duration) Entry {
		entry := asked(question, baseTime.Add(hours*time.Hour))
		entry.Text = bigAnswer(320)
		return entry
	}
	sample := entryOf("q1", 0)
	sample.Identifier = "local-1"
	size := ThreadSize(ToFile([]Entry{sample})[0])
	for _, testCase := range []struct {
		name    string
		bytes   int
		times   []time.Duration
		want    []string
		dropped int
	}{
		// The file limit holds three threads (and the overhead and separators).
		{"the second stored is the oldest", FileOverhead + 3*size + 2 + 10, []time.Duration{2, 0, 1, 3}, []string{"q1", "q3", "q4"}, 1},
		{"two have to go", FileOverhead + 2*size + 1 + 10, []time.Duration{2, 0, 1, 3}, []string{"q1", "q4"}, 2},
		{"all fit", FileOverhead + 4*size + 3 + 10, []time.Duration{2, 0, 1, 3}, []string{"q1", "q2", "q3", "q4"}, 0},
		{"exactly the limit", FileOverhead + 4*size + 3, []time.Duration{2, 0, 1, 3}, []string{"q1", "q2", "q3", "q4"}, 0},
		{"one byte over", FileOverhead + 4*size + 2, []time.Duration{2, 0, 1, 3}, []string{"q1", "q3", "q4"}, 1},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			store := Store{Bytes: testCase.bytes}
			for i, hours := range testCase.times {
				store.Add(entryOf("q"+string(rune('1'+i)), hours))
			}
			if dropped := store.Fit(widgetKey); dropped != testCase.dropped {
				test.Errorf("dropped %d, want %d", dropped, testCase.dropped)
			}
			if diff := cmp.Diff(testCase.want, questions(store.ForRequest(widgetKey))); diff != "" {
				test.Errorf("kept (-want +got):\n%s", diff)
			}
			data, err := Encode(ToFile(store.ForRequest(widgetKey)))
			if err != nil {
				test.Fatal(err)
			}
			if len(data) > testCase.bytes && len(store.ForRequest(widgetKey)) > 1 {
				test.Errorf("file is %d bytes, over the limit %d", len(data), testCase.bytes)
			}
		})
	}
}

// The newest thread is kept even when it alone is over the limit; dropping it
// would lose the answer the user is waiting for.
func TestFitNeverDropsTheLastThread(test *testing.T) {
	store := Store{Bytes: 100}
	entry := asked("only", baseTime)
	entry.Text = bigAnswer(1000)
	store.Add(entry)
	if dropped := store.Fit(widgetKey); dropped != 0 || len(store.ForRequest(widgetKey)) != 1 {
		test.Errorf("dropped %d, %d left", dropped, len(store.ForRequest(widgetKey)))
	}
}

func TestFinish(test *testing.T) {
	var store Store
	identifier := store.Add(asked("why?", baseTime))
	key, found := store.Finish(identifier, github.LocalAnswered, "because")
	if !found || key != widgetKey {
		test.Fatalf("Finish = %v, %v", key, found)
	}
	got := store.ForRequest(widgetKey)[0]
	if got.State != github.LocalAnswered || got.Text != "because" {
		test.Errorf("thread = %+v", got)
	}
	if _, found := store.Finish("local-99", github.LocalFailed, "x"); found {
		test.Error("an unknown identifier was found")
	}
}

func TestMergeContinuesIdentifiersAfterTheLargestLoaded(test *testing.T) {
	var store Store
	store.Merge(widgetKey, []Entry{
		{Identifier: "local-5", Question: "old", State: github.LocalAnswered},
		{Identifier: "local-9", Question: "older", State: github.LocalAnswered},
	})
	if next := store.Add(asked("new", baseTime)); next != "local-10" {
		test.Errorf("next identifier = %q, want local-10", next)
	}
	want := []string{"local-5", "local-9", "local-10"}
	if diff := cmp.Diff(want, identifiers(store.ForRequest(widgetKey))); diff != "" {
		test.Errorf("identifiers (-want +got):\n%s", diff)
	}
}

func TestMergeRenumbersIdentifiersThatClash(test *testing.T) {
	var store Store
	live := store.Add(asked("live", baseTime))
	if live != "local-1" {
		test.Fatalf("live identifier = %q", live)
	}
	changed := store.Merge(widgetKey, []Entry{
		{Identifier: "local-1", Question: "from the file"},
		{Identifier: "../escape", Question: "odd identifier"},
		{Identifier: "local-1", Question: "another", State: github.LocalAnswered}, // same identifier, other question
	})
	if !changed {
		test.Error("renumbering did not count as a change")
	}
	seen := map[string]bool{}
	for _, thread := range store.ForRequest(widgetKey) {
		if seen[thread.Identifier] {
			test.Errorf("identifier %q is used twice", thread.Identifier)
		}
		seen[thread.Identifier] = true
		if _, valid := number(thread.Identifier); !valid {
			test.Errorf("identifier %q is not of the form local-N", thread.Identifier)
		}
	}
	if len(seen) != 4 {
		test.Errorf("%d threads, want 4", len(seen))
	}
	if next := store.Add(asked("next", baseTime)); seen[next] {
		test.Errorf("new identifier %q was already used", next)
	}
}

func TestMergeDoesNotKeepDuplicates(test *testing.T) {
	thread := Entry{Identifier: "local-1", Host: "github.com", Repository: widgets, Number: 7, Question: "why?", Path: "a.go", Asked: baseTime, State: github.LocalAnswered}
	for _, testCase := range []struct {
		name    string
		loaded  []Entry
		stored  int // threads in the store before the merge
		want    int
		changed bool
	}{
		{"already in the store", []Entry{thread}, 1, 1, true},
		{"twice in the file", []Entry{thread, thread}, 0, 1, true},
		{"nothing to merge", nil, 1, 1, false},
		{"a new one", []Entry{thread}, 0, 1, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			var store Store
			if testCase.stored > 0 {
				store.byRequest = map[Key][]Entry{widgetKey: {thread}}
			}
			if changed := store.Merge(widgetKey, testCase.loaded); changed != testCase.changed {
				test.Errorf("changed = %v, want %v", changed, testCase.changed)
			}
			if got := len(store.ForRequest(widgetKey)); got != testCase.want {
				test.Errorf("%d threads, want %d", got, testCase.want)
			}
		})
	}
}

// A thread of another pull request with the same identifier and text is not a
// duplicate.
func TestMergeDuplicateCheckStaysWithinThePullRequest(test *testing.T) {
	var store Store
	thread := Entry{Identifier: "local-1", Question: "why?", Path: "a.go", Asked: baseTime, State: github.LocalAnswered}
	first := Key{Host: "github.com", Repository: widgets, Number: 1}
	second := Key{Host: "github.com", Repository: widgets, Number: 2}
	store.Merge(first, []Entry{thread})
	store.Merge(second, []Entry{thread})
	if got := len(store.ForRequest(second)); got != 1 {
		test.Fatalf("second pull request has %d threads, want 1", got)
	}
	if store.ForRequest(first)[0].Identifier == store.ForRequest(second)[0].Identifier {
		test.Error("both pull requests share an identifier")
	}
}

func TestMergeFailsWhatWasWaiting(test *testing.T) {
	var store Store
	changed := store.Merge(widgetKey, []Entry{{Identifier: "local-1", Question: "why?", State: github.LocalWaiting}})
	got := store.ForRequest(widgetKey)
	if !changed || len(got) != 1 || got[0].State != github.LocalFailed || got[0].Text != ClosedText {
		test.Errorf("changed = %v, threads = %+v", changed, got)
	}
}

func TestMergeAppliesTheLimitToWhatIsLoaded(test *testing.T) {
	store := Store{Limit: 2}
	var loaded []Entry
	for i, question := range []string{"q1", "q2", "q3", "q4"} {
		entry := asked(question, baseTime.Add(time.Duration(i)*time.Hour))
		entry.Identifier = "local-" + string(rune('1'+i))
		entry.State = github.LocalAnswered
		loaded = append(loaded, entry)
	}
	if !store.Merge(widgetKey, loaded) {
		test.Error("dropping threads did not count as a change")
	}
	if diff := cmp.Diff([]string{"q3", "q4"}, questions(store.ForRequest(widgetKey))); diff != "" {
		test.Errorf("kept (-want +got):\n%s", diff)
	}
}

func TestConversionRoundTrip(test *testing.T) {
	entry := Entry{
		Identifier: "local-3", Host: "github.com", Repository: widgets, Number: 7, Head: "abc", Path: "a.go",
		Side: "RIGHT", Line: 5, StartLine: 4, Question: "why?", State: github.LocalAnswered, Text: "because", Asked: baseTime,
	}
	got := FromFile(widgetKey, ToFile([]Entry{entry}))
	if diff := cmp.Diff([]Entry{entry}, got); diff != "" {
		test.Errorf("round trip (-want +got):\n%s", diff)
	}
	if got := FromFile(widgetKey, []Thread{{State: "mystery"}}); len(got) != 0 {
		test.Errorf("a thread in an unknown state was kept: %+v", got)
	}
}
