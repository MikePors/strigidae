package keys

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
)

// Kind classifies the outcome of feeding a key to a Matcher.
type Kind int

const (
	// NoMatch means the key (with any pending prefix) matches nothing.
	NoMatch Kind = iota
	// Pending means the keys so far are a proper prefix of a longer sequence.
	// The caller should start a timeout and call Expire when it fires.
	Pending
	// Matched means a complete binding sequence was recognised.
	Matched
)

// MaximumCount caps a typed count, so a long run of digits cannot overflow.
const MaximumCount = 9999

// Result is the outcome of Feed or Expire. Sequence is the matched sequence
// for Matched (for example "g g"), the pending sequence for Pending, and the
// raw key for NoMatch. Count is the count typed before a Matched sequence
// (0 means none); a count still being typed is Pending with an empty Sequence.
type Result struct {
	Kind     Kind
	Sequence string
	Count    int
}

// Matcher resolves multi-key sequences such as "g g" or "z a b". Sequences are
// written in binding keys as space-separated key names.
//
// A Matcher is pure state: it has no clock. The owner starts a timer when
// Feed returns Pending and calls Expire when it fires, which keeps tests free
// of sleeps.
//
// Digits typed before a sequence form a count ("11j"): 1-9 start it and 0
// continues it. The count is attached to the Matched result and cleared by
// any other outcome (an unbound key, Expire, Reset).
type Matcher struct {
	pending []string
	count   int
}

// Feed adds one key press and resolves it against the bindings.
//
// When the keys so far are a prefix of a longer sequence the result is
// Pending, even if they also match a shorter binding exactly (Expire delivers
// the shorter one). If a pending prefix turns out not to lead anywhere it is
// discarded and the new key is tried on its own.
func (matcher *Matcher) Feed(pressed string, bindings []key.Binding) Result {
	if len(matcher.pending) == 0 && matcher.addDigit(pressed) {
		return Result{Kind: Pending}
	}
	found := len(matcher.pending) > 0
	sequence := strings.Join(append(matcher.pending[:len(matcher.pending):len(matcher.pending)], pressed), " ")
	exact, longer := lookup(sequence, bindings)
	switch {
	case longer:
		matcher.pending = append(matcher.pending, pressed)
		return Result{Kind: Pending, Sequence: sequence}
	case exact:
		count := matcher.count
		matcher.Reset()
		return Result{Kind: Matched, Sequence: sequence, Count: count}
	case found:
		matcher.pending = nil
		if len(pressed) == 1 && pressed[0] >= '0' && pressed[0] <= '9' {
			matcher.count = 0 // the broken prefix ends the old count
		}
		return matcher.Feed(pressed, bindings)
	default:
		matcher.Reset()
		return Result{Kind: NoMatch, Sequence: pressed}
	}
}

// Expire is called when the sequence timeout fires. If the pending keys are a
// complete binding it returns Matched; otherwise NoMatch. Either way the
// pending state is cleared.
func (matcher *Matcher) Expire(bindings []key.Binding) Result {
	sequence := strings.Join(matcher.pending, " ")
	count := matcher.count
	matcher.Reset()
	if sequence == "" {
		return Result{Kind: NoMatch}
	}
	if exact, _ := lookup(sequence, bindings); exact {
		return Result{Kind: Matched, Sequence: sequence, Count: count}
	}
	return Result{Kind: NoMatch, Sequence: sequence}
}

// Pending reports whether a sequence or a count is in progress.
func (matcher *Matcher) Pending() bool { return len(matcher.pending) > 0 || matcher.count > 0 }

// Display returns the count and pending keys as typed, for the status bar
// ("11", "g", "3g").
func (matcher *Matcher) Display() string {
	display := strings.Join(matcher.pending, "")
	if matcher.count > 0 {
		display = strconv.Itoa(matcher.count) + display
	}
	return display
}

// Reset discards any pending keys and count.
func (matcher *Matcher) Reset() {
	matcher.pending = nil
	matcher.count = 0
}

// addDigit extends the count when pressed is a digit that may be part of one:
// 1-9 always, 0 only after another digit.
func (matcher *Matcher) addDigit(pressed string) bool {
	if len(pressed) != 1 || pressed[0] < '0' || pressed[0] > '9' || (pressed == "0" && matcher.count == 0) {
		return false
	}
	matcher.count = min(matcher.count*10+int(pressed[0]-'0'), MaximumCount)
	return true
}

func lookup(sequence string, bindings []key.Binding) (exact, longer bool) {
	prefix := sequence + " "
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		for _, pressed := range binding.Keys() {
			if pressed == sequence {
				exact = true
			} else if strings.HasPrefix(pressed, prefix) {
				longer = true
			}
		}
	}
	return exact, longer
}
