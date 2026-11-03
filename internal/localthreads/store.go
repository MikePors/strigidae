package localthreads

import (
	"slices"
	"strconv"
	"strings"

	"github.com/MikePors/strigidae/internal/github"
)

// DefaultLimit is the most local threads kept for one pull request. When
// another is asked, the oldest goes.
const DefaultLimit = 200

// ClosedText is why a thread that was still waiting when strig exited has no
// answer.
const ClosedText = "strig was closed"

const identifierPrefix = "local-"

// Store holds the local threads for the life of strig. It is not safe for
// concurrent use: the caller owns it and copies out what other goroutines need.
type Store struct {
	// Limit is the most threads kept per pull request; 0 means DefaultLimit.
	Limit int
	// Bytes is the largest a pull request's file may be; 0 means MaxFileSize.
	Bytes int

	next      int
	byRequest map[Key][]Entry
}

func (store *Store) maximum() int {
	if store.Limit > 0 {
		return store.Limit
	}
	return DefaultLimit
}

func (store *Store) maximumBytes() int {
	if store.Bytes > 0 {
		return store.Bytes
	}
	return MaxFileSize
}

// Fit drops the oldest threads of key, by asked time as the count limit does,
// until the file they make is no bigger than the file size limit (a bigger one
// would be refused when read back), and says how many went. The newest thread
// is never dropped: one answer is at most plugins.MaximumOutputSize, which
// is far below the limit.
func (store *Store) Fit(key Key) (dropped int) {
	threads := store.byRequest[key]
	sizes := make([]int, len(threads))
	total := FileOverhead + max(len(threads)-1, 0)
	for i, thread := range threads {
		sizes[i] = ThreadSize(ToFile([]Entry{thread})[0])
		total += sizes[i]
	}
	for total > store.maximumBytes() && len(threads) > 1 {
		oldest := oldestIndex(threads)
		total -= sizes[oldest] + 1
		threads = slices.Delete(threads, oldest, oldest+1)
		sizes = slices.Delete(sizes, oldest, oldest+1)
		dropped++
	}
	store.byRequest[key] = threads
	return dropped
}

// oldestIndex is the position of the thread asked first; of equal times, the
// one stored first.
func oldestIndex(threads []Entry) int {
	oldest := 0
	for i := range threads {
		if threads[i].Asked.Before(threads[oldest].Asked) {
			oldest = i
		}
	}
	return oldest
}

// newIdentifier returns an identifier no thread has had in this run or in a
// file loaded in it.
func (store *Store) newIdentifier() string {
	store.next++
	return identifierPrefix + strconv.Itoa(store.next)
}

// Add stores a new thread and gives it its identifier. Over the limit, the
// oldest thread of its pull request is dropped.
func (store *Store) Add(entry Entry) string {
	if store.byRequest == nil {
		store.byRequest = map[Key][]Entry{}
	}
	entry.Identifier = store.newIdentifier()
	key := entry.Key()
	store.byRequest[key] = append(store.byRequest[key], entry)
	store.trim(key)
	return entry.Identifier
}

// trim drops the oldest threads of key until it is within the limit, and
// reports whether any went. Of equal times the one stored first goes.
func (store *Store) trim(key Key) (dropped bool) {
	for len(store.byRequest[key]) > store.maximum() {
		threads := store.byRequest[key]
		oldest := oldestIndex(threads)
		store.byRequest[key] = slices.Delete(threads, oldest, oldest+1)
		dropped = true
	}
	return dropped
}

// Finish records how a thread's question ended, and says which pull request
// it belongs to.
func (store *Store) Finish(identifier string, state github.LocalState, text string) (key Key, found bool) {
	for key, threads := range store.byRequest {
		for i := range threads {
			if threads[i].Identifier == identifier {
				store.byRequest[key][i].State, store.byRequest[key][i].Text = state, text
				return key, true
			}
		}
	}
	return Key{}, false
}

// Snapshot copies every thread.
func (store *Store) Snapshot() []Entry {
	var all []Entry
	for _, threads := range store.byRequest {
		all = append(all, threads...)
	}
	return all
}

// ForRequest copies the threads of one pull request.
func (store *Store) ForRequest(key Key) []Entry {
	return slices.Clone(store.byRequest[key])
}

// find returns the thread with identifier, if any pull request has it.
func (store *Store) find(identifier string) (Entry, bool) {
	for _, threads := range store.byRequest {
		for _, thread := range threads {
			if thread.Identifier == identifier {
				return thread, true
			}
		}
	}
	return Entry{}, false
}

// number is the number in an identifier of the form local-N.
func number(identifier string) (int, bool) {
	text, ok := strings.CutPrefix(identifier, identifierPrefix)
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 || strconv.Itoa(value) != text {
		return 0, false
	}
	return value, true
}

// Merge adds the threads read from a file to the pull request's own, which
// are normally none. A thread already in the store (same identifier and same
// question) is not added twice. A loaded identifier that is not of the form
// local-N, or that another thread already has, is replaced by a new one, and
// numbering continues after the largest loaded number, so identifiers stay
// unique across restarts. A thread that was waiting when strig exited has no
// answer now. It reports whether the result differs from the file, so the
// caller writes it back.
func (store *Store) Merge(key Key, loaded []Entry) (changed bool) {
	if store.byRequest == nil {
		store.byRequest = map[Key][]Entry{}
	}
	for _, thread := range loaded {
		if value, valid := number(thread.Identifier); valid {
			store.next = max(store.next, value)
		}
	}
	merged := make([]Entry, 0, len(loaded)+len(store.byRequest[key]))
	for _, thread := range loaded {
		_, taken := store.find(thread.Identifier)
		// Only this pull request's threads can be the same thread; another's
		// with the same identifier is a clash, renumbered below.
		duplicate := func(other Entry) bool {
			return other.Identifier == thread.Identifier && sameQuestion(other, thread)
		}
		if slices.ContainsFunc(store.byRequest[key], duplicate) || slices.ContainsFunc(merged, duplicate) {
			changed = true // a duplicate is not kept
			continue
		}
		_, valid := number(thread.Identifier)
		taken = taken || slices.ContainsFunc(merged, func(other Entry) bool { return other.Identifier == thread.Identifier })
		if !valid || taken {
			thread.Identifier, changed = store.newIdentifier(), true
		}
		if thread.State == github.LocalWaiting {
			thread.State, thread.Text, changed = github.LocalFailed, ClosedText, true
		}
		merged = append(merged, thread)
	}
	store.byRequest[key] = append(merged, store.byRequest[key]...)
	if store.trim(key) {
		changed = true
	}
	return changed
}

func sameQuestion(first, second Entry) bool {
	return first.Question == second.Question && first.Path == second.Path && first.Asked.Equal(second.Asked)
}
