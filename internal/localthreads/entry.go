package localthreads

import (
	"time"

	"github.com/MikePors/strigidae/internal/github"
)

// Key is what local threads are stored by: the host, the repository and the
// pull request number. The head is on each entry.
type Key struct {
	Host       string
	Repository github.Repository
	Number     int
}

// Entry is a plugin's question about diff lines and what came of it. It lives
// in a Store, and in a file between runs of strig; Thread is its form in the
// file.
type Entry struct {
	// Identifier is unique among local threads; the views use it as a key
	// for folds. Nothing may treat it as telling a local thread from a
	// GitHub one: that is github.ReviewThread.Local.
	Identifier string
	// Host is the GitHub host of the pull request; the views do not look at it.
	Host       string
	Repository github.Repository
	Number     int
	// Head is the head commit the question was asked on. Whether the thread
	// is outdated is worked out against the diff on screen, not stored.
	Head      string
	Path      string
	Side      github.DiffSide
	Line      int
	StartLine int
	Question  string
	State     github.LocalState
	// Text is the answer once State is LocalAnswered, and why there is none
	// once it is LocalFailed. It is the plugin's text and has not been cleaned.
	Text string
	// Asked is when the question was asked; it picks the thread to drop when
	// there are too many, and is not shown.
	Asked time.Time
}

// Key is the pull request the entry belongs to.
func (entry Entry) Key() Key {
	return Key{entry.Host, entry.Repository, entry.Number}
}

// ToFile returns the form of entries that a file holds.
func ToFile(entries []Entry) []Thread {
	saved := make([]Thread, 0, len(entries))
	for _, entry := range entries {
		saved = append(saved, Thread{
			Identifier: entry.Identifier, Head: entry.Head, Path: entry.Path, Side: entry.Side,
			Line: entry.Line, StartLine: entry.StartLine, Question: entry.Question,
			State: stateName(entry.State), Text: entry.Text, Asked: entry.Asked,
		})
	}
	return saved
}

// FromFile turns what a file held into entries of one pull request. A thread
// in a state this version doesn't know is left out.
func FromFile(key Key, saved []Thread) []Entry {
	entries := make([]Entry, 0, len(saved))
	for _, thread := range saved {
		state, ok := stateFromName(thread.State)
		if !ok {
			continue
		}
		entries = append(entries, Entry{
			Identifier: thread.Identifier, Host: key.Host, Repository: key.Repository, Number: key.Number,
			Head: thread.Head, Path: thread.Path, Side: thread.Side, Line: thread.Line, StartLine: thread.StartLine,
			Question: thread.Question, State: state, Text: thread.Text, Asked: thread.Asked,
		})
	}
	return entries
}

func stateName(state github.LocalState) string {
	switch state {
	case github.LocalAnswered:
		return StateAnswered
	case github.LocalFailed:
		return StateFailed
	}
	return StateWaiting
}

func stateFromName(name string) (github.LocalState, bool) {
	switch name {
	case StateWaiting:
		return github.LocalWaiting, true
	case StateAnswered:
		return github.LocalAnswered, true
	case StateFailed:
		return github.LocalFailed, true
	}
	return github.NotLocal, false
}
