package userinterface

import (
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/localthreads"
)

// PullRequestOpenedMessage tells the root that a view has loaded a pull request.
// The first time it hears of one in a run, the root reads its saved local
// threads. Identity.Host is the pull request's host; with none, nothing is kept.
type PullRequestOpenedMessage struct{ Identity PullRequestIdentity }

type (
	localThreadsLoadedMessage struct {
		key     pullRequestKey
		threads []LocalThread
		err     error
	}
	localThreadsSavedMessage struct {
		key      pullRequestKey
		setAside bool
		err      error
	}
)

// localSave is where one pull request's file stands. At most one operation on
// it (the first read, or a write) is in flight; a change that comes meanwhile
// only sets pending, and one write of the latest threads follows. So an older
// copy can never land after a newer one.
type localSave struct {
	inFlight bool
	pending  bool
}

// localThreadFiles is the root's bookkeeping for the files. Only Update uses
// it, apart from the wait group, which commands use.
type localThreadFiles struct {
	directory string // the state directory; empty keeps nothing
	opened    map[pullRequestKey]bool
	saves     map[pullRequestKey]*localSave
	setAside  map[pullRequestKey]bool // the file is unreadable: move it before writing
	explained map[pullRequestKey]bool // why nothing is kept was logged
	running   sync.WaitGroup          // read and write commands that have not returned; see Close
}

func (files *localThreadFiles) save(key pullRequestKey) *localSave {
	if files.saves == nil {
		files.saves = map[pullRequestKey]*localSave{}
	}
	if files.saves[key] == nil {
		files.saves[key] = &localSave{}
	}
	return files.saves[key]
}

// localThreadPath is the file of a pull request. When there can be none (no
// state directory, no host, a name that isn't plain) it says so once in the log
// and the threads live for the session only.
func (model *Model) localThreadPath(key pullRequestKey) (string, bool) {
	files := &model.localFiles
	if files.directory == "" {
		return "", false
	}
	path, err := localthreads.Path(files.directory, key.host, key.repository.Owner, key.repository.Name, key.number)
	if err != nil {
		if files.explained == nil {
			files.explained = map[pullRequestKey]bool{}
		}
		if !files.explained[key] {
			files.explained[key] = true
			model.log.Warn("local threads are not kept for this pull request", "reason", err.Error(), "number", key.number, "host empty", key.host == "")
		}
		return "", false
	}
	return path, true
}

// pullRequestOpened reads the saved threads the first time a pull request is
// opened. Until the read returns it counts as a write in flight, so nothing
// overwrites the file before its threads are loaded.
func (model *Model) pullRequestOpened(message PullRequestOpenedMessage) tea.Cmd {
	key := pullRequestKey{message.Identity.Host, message.Identity.Repository, message.Identity.Number}
	files := &model.localFiles
	if model.quitting || files.opened[key] {
		return nil
	}
	path, ok := model.localThreadPath(key)
	if !ok {
		return nil
	}
	if files.opened == nil {
		files.opened = map[pullRequestKey]bool{}
	}
	files.opened[key] = true
	files.save(key).inFlight = true
	files.running.Add(1)
	return func() tea.Msg {
		defer files.running.Done()
		file, err := localthreads.Read(path)
		return localThreadsLoadedMessage{key: key, threads: fromFile(key, file.Threads), err: err}
	}
}

// localThreadsLoaded merges what the file held. A file that can't be used is
// logged, without its content, and set aside before the next write.
func (model *Model) localThreadsLoaded(message localThreadsLoadedMessage) tea.Cmd {
	files := &model.localFiles
	save := files.save(message.key)
	save.inFlight = false
	changed := false
	if message.err != nil {
		model.log.Warn("local threads file ignored", "reason", loadFailure(message.err), "number", message.key.number)
		if files.setAside == nil {
			files.setAside = map[pullRequestKey]bool{}
		}
		files.setAside[message.key] = true
	} else {
		changed = model.localThreads.merge(message.key, message.threads)
	}
	if changed {
		save.pending = true
	}
	var written tea.Cmd
	if save.pending {
		save.pending = false
		written = model.writeLocalThreads(message.key)
	}
	if message.err != nil || len(message.threads) == 0 {
		return written
	}
	return tea.Batch(written, model.broadcast(LocalThreadsMessage{Threads: model.localThreads.snapshot()}))
}

// loadFailure names why a file wasn't used, in words that hold none of its content.
func loadFailure(err error) string {
	switch {
	case errors.Is(err, localthreads.ErrTooLarge):
		return "too large"
	case errors.Is(err, localthreads.ErrUnknownVersion):
		return "unknown version"
	case errors.Is(err, localthreads.ErrCorrupt):
		return "can't be decoded"
	}
	return "can't be read"
}

// saveLocalThreads writes the pull request's threads after a change: at once
// if nothing is in flight for it, else when what is in flight ends.
func (model *Model) saveLocalThreads(key pullRequestKey) tea.Cmd {
	if _, ok := model.localThreadPath(key); !ok {
		return nil
	}
	save := model.localFiles.save(key)
	if save.inFlight {
		save.pending = true
		return nil
	}
	return model.writeLocalThreads(key)
}

// writeLocalThreads starts a write of the threads as they are now.
func (model *Model) writeLocalThreads(key pullRequestKey) tea.Cmd {
	path, ok := model.localThreadPath(key)
	if !ok {
		return nil
	}
	files := &model.localFiles
	files.save(key).inFlight = true
	threads := toFile(model.localThreads.forRequest(key))
	setAside := files.setAside[key]
	files.running.Add(1)
	return func() tea.Msg {
		defer files.running.Done()
		return localThreadsSavedMessage{key: key, setAside: setAside, err: writeLocalFile(path, threads, setAside)}
	}
}

// writeLocalFile moves an unreadable file aside if asked, then replaces the
// file. If the file can't be moved it is not overwritten.
func writeLocalFile(path string, threads []localthreads.Thread, setAside bool) error {
	data, err := localthreads.Encode(threads)
	if err != nil {
		return err
	}
	if setAside {
		if err := localthreads.SetAside(path); err != nil {
			return err
		}
	}
	return localthreads.Write(path, data)
}

// localThreadsSaved ends a write. A failure is logged, without any thread
// text, and stops nothing. A change that came meanwhile is written now.
func (model *Model) localThreadsSaved(message localThreadsSavedMessage) tea.Cmd {
	files := &model.localFiles
	save := files.save(message.key)
	save.inFlight = false
	if message.err != nil {
		model.log.Warn("local threads not saved", "err", message.err, "number", message.key.number)
	} else if message.setAside {
		delete(files.setAside, message.key)
	}
	if !save.pending {
		return nil
	}
	save.pending = false
	return model.writeLocalThreads(message.key)
}

// flushLocalThreads writes, from Close, the changes that were waiting for a
// write in flight. The caller has seen the writes in flight end; if they did
// not end in time, an older copy might still land, so it does not call this.
func (model *Model) flushLocalThreads() {
	files := &model.localFiles
	for key, save := range files.saves {
		if !save.pending {
			continue
		}
		save.pending = false
		path, ok := model.localThreadPath(key)
		if !ok {
			continue
		}
		if err := writeLocalFile(path, toFile(model.localThreads.forRequest(key)), files.setAside[key]); err != nil {
			model.log.Warn("local threads not saved", "err", err, "number", key.number)
		}
	}
}

func toFile(threads []LocalThread) []localthreads.Thread {
	saved := make([]localthreads.Thread, 0, len(threads))
	for _, thread := range threads {
		saved = append(saved, localthreads.Thread{
			Identifier: thread.Identifier, Head: thread.Head, Path: thread.Path, Side: thread.Side,
			Line: thread.Line, StartLine: thread.StartLine, Question: thread.Question,
			State: stateName(thread.State), Text: thread.Text, Asked: thread.Asked,
		})
	}
	return saved
}

// fromFile turns what a file held into threads of one pull request. A thread
// in a state this version doesn't know is left out.
func fromFile(key pullRequestKey, saved []localthreads.Thread) []LocalThread {
	threads := make([]LocalThread, 0, len(saved))
	for _, thread := range saved {
		state, ok := stateFromName(thread.State)
		if !ok {
			continue
		}
		threads = append(threads, LocalThread{
			Identifier: thread.Identifier, Host: key.host, Repository: key.repository, Number: key.number,
			Head: thread.Head, Path: thread.Path, Side: thread.Side, Line: thread.Line, StartLine: thread.StartLine,
			Question: thread.Question, State: state, Text: thread.Text, Asked: thread.Asked,
		})
	}
	return threads
}

func stateName(state github.LocalState) string {
	switch state {
	case github.LocalAnswered:
		return localthreads.StateAnswered
	case github.LocalFailed:
		return localthreads.StateFailed
	}
	return localthreads.StateWaiting
}

func stateFromName(name string) (github.LocalState, bool) {
	switch name {
	case localthreads.StateWaiting:
		return github.LocalWaiting, true
	case localthreads.StateAnswered:
		return github.LocalAnswered, true
	case localthreads.StateFailed:
		return github.LocalFailed, true
	}
	return github.NotLocal, false
}
