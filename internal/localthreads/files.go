package localthreads

import (
	"errors"
	"io"
	"log/slog"
	"sync"
)

// Decision is what a change to a pull request's threads asks of the caller.
type Decision int

const (
	// Wait means an operation on the file is in flight; one write of the latest
	// threads follows when it ends.
	Wait Decision = iota
	// ReadFirst means the saved threads have not been read yet. The caller starts
	// the read (BeginRead); the write follows it.
	ReadFirst
	// WriteNow means nothing is in flight. The caller starts a write (BeginWrite).
	WriteNow
)

// save is where one pull request's file stands. At most one operation on it
// (the first read, or a write) is in flight; a change that comes meanwhile
// only sets pending, and one write of the latest threads follows.
type save struct {
	inFlight bool
	pending  bool
}

// Files is the bookkeeping for the files of every pull request: which were
// read, which operation is in flight, and which copy was written last. It
// decides; the caller runs the reads and writes in commands and tells Files
// how they ended. Only the caller's update goroutine uses the fields above the
// mutex; the fields below it are shared with the commands and the flush, and
// are used only with the mutex held.
//
// Every change of a pull request's threads bumps its generation, and a write
// carries the generation it was made from. A write whose generation is not
// newer than the last one written is skipped, so an older copy can never land
// after a newer one, whoever writes (a command that ran late, or the flush).
type Files struct {
	// Directory is the state directory; empty keeps nothing.
	Directory string
	// Logger gets the warnings; none logs nothing. They never hold thread text.
	Logger *slog.Logger

	opened     map[Key]bool
	loaded     map[Key]bool // the saved threads have been merged
	saves      map[Key]*save
	generation map[Key]int
	explained  map[Key]bool // why nothing is kept was logged

	flushed <-chan struct{}

	mutex      sync.Mutex
	written    map[Key]int  // the generation last written
	unreadable map[Key]bool // the file can't be used: move it aside before writing
}

func (files *Files) logger() *slog.Logger {
	if files.Logger == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return files.Logger
}

func (files *Files) save(key Key) *save {
	if files.saves == nil {
		files.saves = map[Key]*save{}
	}
	if files.saves[key] == nil {
		files.saves[key] = &save{}
	}
	return files.saves[key]
}

// Path is the file of a pull request. When there can be none (no state
// directory, no host, a name that isn't plain) it says so once in the log and
// the threads live for the session only.
func (files *Files) Path(key Key) (string, bool) {
	if files.Directory == "" {
		return "", false
	}
	path, err := Path(files.Directory, key.Host, key.Repository.Owner, key.Repository.Name, key.Number)
	if err != nil {
		if files.explained == nil {
			files.explained = map[Key]bool{}
		}
		if !files.explained[key] {
			files.explained[key] = true
			files.logger().Warn("local threads are not kept for this pull request", "reason", err.Error(), "number", key.Number, "host empty", key.Host == "")
		}
		return "", false
	}
	return path, true
}

// BeginRead starts the read of a pull request's file, once per run, and
// returns its path. Until EndRead it counts as a write in flight, so nothing
// overwrites the file before its threads are loaded. It says false when the
// file was already opened or there is none.
func (files *Files) BeginRead(key Key) (path string, ok bool) {
	if files.opened[key] {
		return "", false
	}
	path, ok = files.Path(key)
	if !ok {
		return "", false
	}
	if files.opened == nil {
		files.opened = map[Key]bool{}
	}
	files.opened[key] = true
	files.save(key).inFlight = true
	return path, true
}

// Load reads the threads of a pull request's file. It is for the command that
// BeginRead started.
func Load(path string, key Key) ([]Entry, error) {
	file, err := Read(path)
	return FromFile(key, file.Threads), err
}

// EndRead ends the read. err is the read's failure, or nil; changed is whether
// merging what it held changed the store (see Store.Merge), which a failed read
// never does. A file that can't be used is logged, without its content, and
// set aside before the next write. It says whether a write is due: the merge
// changed something, or a change came meanwhile.
func (files *Files) EndRead(key Key, err error, changed bool) (write bool) {
	current := files.save(key)
	current.inFlight = false
	if files.loaded == nil {
		files.loaded = map[Key]bool{}
	}
	files.loaded[key] = true
	if err != nil {
		files.logger().Warn("local threads file ignored", "reason", loadFailure(err), "number", key.Number)
		files.markUnreadable(key)
	}
	if changed {
		files.generation = bump(files.generation, key)
		current.pending = true
	}
	if current.pending {
		current.pending = false
		return true
	}
	return false
}

// Changed records that the threads of key differ from the last write, and
// says what to do about it.
func (files *Files) Changed(key Key) Decision {
	files.generation = bump(files.generation, key)
	current := files.save(key)
	switch {
	case !files.opened[key]:
		current.pending = true
		return ReadFirst
	case current.inFlight:
		current.pending = true
		return Wait
	}
	return WriteNow
}

func bump(generation map[Key]int, key Key) map[Key]int {
	if generation == nil {
		generation = map[Key]int{}
	}
	generation[key]++
	return generation
}

// BeginWrite starts a write of the threads as they are now: it returns the
// path and the generation the copy is made from, to give to Write.
func (files *Files) BeginWrite(key Key) (path string, generation int, ok bool) {
	path, ok = files.Path(key)
	if !ok {
		return "", 0, false
	}
	files.save(key).inFlight = true
	return path, files.generation[key], true
}

// EndWrite ends a write. A failure is logged, without any thread text, and
// stops nothing. It says whether a change came meanwhile, so a write of the
// latest copy is due.
func (files *Files) EndWrite(key Key, err error) (again bool) {
	current := files.save(key)
	current.inFlight = false
	if err != nil {
		files.logger().Warn("local threads not saved", "err", err, "number", key.Number)
	}
	if !current.pending {
		return false
	}
	current.pending = false
	return true
}

// Flushed is closed when the writes of the last Flush end.
func (files *Files) Flushed() <-chan struct{} { return files.flushed }

// HoldForTest takes the lock every write takes, as a stuck disk would, and
// returns the function that gives it back. It exists for tests of the caller's
// quit wait and must never be called otherwise. Holding it blocks Write and an
// EndRead that carries an error, so the holder must not call them.
func (files *Files) HoldForTest() (release func()) {
	files.mutex.Lock()
	var once sync.Once
	return func() { once.Do(files.mutex.Unlock) }
}

func (files *Files) markUnreadable(key Key) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	if files.unreadable == nil {
		files.unreadable = map[Key]bool{}
	}
	files.unreadable[key] = true
}

func (files *Files) writtenAtLeast(key Key, generation int) bool {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	return generation <= files.written[key]
}

// Write replaces the file with threads, made from the given generation, unless
// a newer copy has been written. An unreadable file is moved aside first, once,
// and if that fails nothing is overwritten.
func (files *Files) Write(key Key, path string, generation int, threads []Thread) error {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	if generation <= files.written[key] {
		return nil
	}
	data, err := Encode(threads)
	if err != nil {
		return err
	}
	if files.unreadable[key] {
		if err := SetAside(path); err != nil {
			return err
		}
		delete(files.unreadable, key)
	}
	if err := Write(path, data); err != nil {
		return err
	}
	if files.written == nil {
		files.written = map[Key]int{}
	}
	files.written[key] = generation
	return nil
}

// loadFailure names why a file wasn't used, in words that hold none of its content.
func loadFailure(err error) string {
	switch {
	case errors.Is(err, ErrTooLarge):
		return "too large"
	case errors.Is(err, ErrUnknownVersion):
		return "unknown version"
	case errors.Is(err, ErrCorrupt):
		return "can't be decoded"
	}
	return "can't be read"
}

// flushJob is one pull request whose threads changed since its last write.
type flushJob struct {
	key        Key
	path       string
	generation int
	threads    []Entry
	loaded     bool
	scratch    *Store // for a file never read: the limits to merge it under
}

// Flush starts, for the quit, the write of every pull request whose threads
// changed since the last write that completed. It does not depend on any
// command having run: a write command that never started is made good here. A
// pull request whose saved threads were never read is read first and merged,
// so they are kept. The caller may use the store here; everything that touches
// the files, the mutex included, runs in the goroutine, because a stalled
// write holds the mutex and must not hold the caller. The returned channel
// closes when the writes end, and the caller bounds the wait so a stuck disk
// can't hold quit.
func (files *Files) Flush(store *Store) <-chan struct{} {
	var jobs []flushJob
	for key, generation := range files.generation {
		path, ok := files.Path(key)
		if !ok {
			continue
		}
		job := flushJob{key: key, path: path, generation: generation, threads: store.ForRequest(key), loaded: files.loaded[key]}
		if !job.loaded {
			job.scratch = &Store{Limit: store.Limit, Bytes: store.Bytes, next: store.next, byRequest: map[Key][]Entry{key: job.threads}}
		}
		jobs = append(jobs, job)
	}
	logger := files.logger()
	finished := make(chan struct{})
	files.flushed = finished
	go func() {
		defer close(finished)
		for _, pending := range jobs {
			if files.writtenAtLeast(pending.key, pending.generation) {
				continue
			}
			threads := pending.threads
			if !pending.loaded {
				threads = files.mergeUnread(pending)
			}
			if err := files.Write(pending.key, pending.path, pending.generation, ToFile(threads)); err != nil {
				logger.Warn("local threads not saved", "err", err, "number", pending.key.Number)
			}
		}
	}()
	return finished
}

// mergeUnread reads the file of a pull request whose read never finished and
// merges it with the threads asked this run, as a load would.
func (files *Files) mergeUnread(job flushJob) []Entry {
	file, err := Read(job.path)
	if err != nil {
		files.logger().Warn("local threads file ignored", "reason", loadFailure(err), "number", job.key.Number)
		files.markUnreadable(job.key)
		return job.threads
	}
	job.scratch.Merge(job.key, FromFile(job.key, file.Threads))
	job.scratch.Fit(job.key)
	return job.scratch.ForRequest(job.key)
}
