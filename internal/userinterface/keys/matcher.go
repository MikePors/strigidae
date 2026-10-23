package keys

import (
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

// Result is the outcome of Feed or Expire. Seq is the matched sequence for
// Matched (for example "g g"), the pending sequence for Pending, and the raw
// key for NoMatch.
type Result struct {
	Kind     Kind
	Sequence string
}

// Matcher resolves multi-key sequences such as "g g" or "z a b". Sequences are
// written in binding keys as space-separated key names.
//
// A Matcher is pure state: it has no clock. The owner starts a timer when
// Feed returns Pending and calls Expire when it fires, which keeps tests free
// of sleeps.
type Matcher struct {
	pending []string
}

// Feed adds one key press and resolves it against the bindings.
//
// When the keys so far are a prefix of a longer sequence the result is
// Pending, even if they also match a shorter binding exactly (Expire delivers
// the shorter one). If a pending prefix turns out not to lead anywhere it is
// discarded and the new key is tried on its own.
func (matcher *Matcher) Feed(key string, bindings []key.Binding) Result {
	found := len(matcher.pending) > 0
	sequence := strings.Join(append(matcher.pending[:len(matcher.pending):len(matcher.pending)], key), " ")
	exact, longer := lookup(sequence, bindings)
	switch {
	case longer:
		matcher.pending = append(matcher.pending, key)
		return Result{Kind: Pending, Sequence: sequence}
	case exact:
		matcher.Reset()
		return Result{Kind: Matched, Sequence: sequence}
	case found:
		matcher.Reset()
		return matcher.Feed(key, bindings)
	default:
		return Result{Kind: NoMatch, Sequence: key}
	}
}

// Expire is called when the sequence timeout fires. If the pending keys are a
// complete binding it returns Matched; otherwise NoMatch. Either way the
// pending state is cleared.
func (matcher *Matcher) Expire(bindings []key.Binding) Result {
	sequence := strings.Join(matcher.pending, " ")
	matcher.Reset()
	if sequence == "" {
		return Result{Kind: NoMatch}
	}
	if exact, _ := lookup(sequence, bindings); exact {
		return Result{Kind: Matched, Sequence: sequence}
	}
	return Result{Kind: NoMatch, Sequence: sequence}
}

// Pending reports whether a sequence is in progress.
func (matcher *Matcher) Pending() bool { return len(matcher.pending) > 0 }

// Display returns the pending keys as typed, for the status bar ("g", "]").
func (matcher *Matcher) Display() string { return strings.Join(matcher.pending, "") }

// Reset discards any pending keys.
func (matcher *Matcher) Reset() { matcher.pending = nil }

func lookup(sequence string, bindings []key.Binding) (exact, longer bool) {
	prefix := sequence + " "
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		for _, key := range binding.Keys() {
			if key == sequence {
				exact = true
			} else if strings.HasPrefix(key, prefix) {
				longer = true
			}
		}
	}
	return exact, longer
}
