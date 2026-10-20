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
	Kind Kind
	Seq  string
}

// Matcher resolves multi-key sequences such as "g g" or "] f". Sequences are
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
func (m *Matcher) Feed(k string, bindings []key.Binding) Result {
	had := len(m.pending) > 0
	seq := strings.Join(append(m.pending[:len(m.pending):len(m.pending)], k), " ")
	exact, longer := lookup(seq, bindings)
	switch {
	case longer:
		m.pending = append(m.pending, k)
		return Result{Kind: Pending, Seq: seq}
	case exact:
		m.Reset()
		return Result{Kind: Matched, Seq: seq}
	case had:
		m.Reset()
		return m.Feed(k, bindings)
	default:
		return Result{Kind: NoMatch, Seq: k}
	}
}

// Expire is called when the sequence timeout fires. If the pending keys are a
// complete binding it returns Matched; otherwise NoMatch. Either way the
// pending state is cleared.
func (m *Matcher) Expire(bindings []key.Binding) Result {
	seq := strings.Join(m.pending, " ")
	m.Reset()
	if seq == "" {
		return Result{Kind: NoMatch}
	}
	if exact, _ := lookup(seq, bindings); exact {
		return Result{Kind: Matched, Seq: seq}
	}
	return Result{Kind: NoMatch, Seq: seq}
}

// Pending reports whether a sequence is in progress.
func (m *Matcher) Pending() bool { return len(m.pending) > 0 }

// Display returns the pending keys as typed, for the status bar ("g", "]").
func (m *Matcher) Display() string { return strings.Join(m.pending, "") }

// Reset discards any pending keys.
func (m *Matcher) Reset() { m.pending = nil }

func lookup(seq string, bindings []key.Binding) (exact, longer bool) {
	prefix := seq + " "
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		for _, k := range b.Keys() {
			if k == seq {
				exact = true
			} else if strings.HasPrefix(k, prefix) {
				longer = true
			}
		}
	}
	return exact, longer
}
