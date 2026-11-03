package userinterface

import (
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/localthreads"
)

// The rules for local threads (limits, merging, which write is due, what the
// quit writes) are in internal/localthreads. This file is the glue: it turns
// those decisions into commands and the commands' results into messages.

// PullRequestOpenedMessage tells the root that a view has loaded a pull request.
// The first time it hears of one in a run, the root reads its saved local
// threads. Identity.Host is the pull request's host; with none, nothing is kept.
type PullRequestOpenedMessage struct{ Identity PullRequestIdentity }

type (
	localThreadsLoadedMessage struct {
		key     localthreads.Key
		threads []LocalThread
		err     error
	}
	localThreadsSavedMessage struct {
		key localthreads.Key
		err error
	}
)

// pullRequestOpened reads the saved threads the first time a pull request is
// opened.
func (model *Model) pullRequestOpened(message PullRequestOpenedMessage) tea.Cmd {
	if model.quitting {
		return nil
	}
	return model.readLocalThreads(localthreads.Key{Host: message.Identity.Host, Repository: message.Identity.Repository, Number: message.Identity.Number})
}

// readLocalThreads starts the read of a pull request's file, once per run.
func (model *Model) readLocalThreads(key localthreads.Key) tea.Cmd {
	path, ok := model.localFiles.BeginRead(key)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		threads, err := localthreads.Load(path, key)
		return localThreadsLoadedMessage{key: key, threads: threads, err: err}
	}
}

// localThreadsLoaded merges what the file held.
func (model *Model) localThreadsLoaded(message localThreadsLoadedMessage) tea.Cmd {
	changed := false
	if message.err == nil {
		changed = model.localThreads.Merge(message.key, message.threads)
	}
	var fitted tea.Cmd
	if changed {
		fitted = model.fitLocalThreads(message.key)
	}
	var written tea.Cmd
	if model.localFiles.EndRead(message.key, message.err, changed) {
		written = model.writeLocalThreads(message.key)
	}
	if message.err != nil || len(message.threads) == 0 {
		return tea.Batch(written, fitted)
	}
	return tea.Batch(written, model.broadcast(LocalThreadsMessage{Threads: model.localThreads.Snapshot()}))
}

// saveLocalThreads writes the pull request's threads after a change: at once
// if nothing is in flight for it, else when what is in flight ends.
func (model *Model) saveLocalThreads(key localthreads.Key) tea.Cmd {
	if _, ok := model.localFiles.Path(key); !ok {
		return nil
	}
	decision := model.localFiles.Changed(key)
	fitted := model.fitLocalThreads(key)
	switch decision {
	case localthreads.ReadFirst:
		return tea.Batch(fitted, model.readLocalThreads(key))
	case localthreads.WriteNow:
		return tea.Batch(fitted, model.writeLocalThreads(key))
	}
	return fitted
}

// fitLocalThreads keeps the pull request's file small enough to be read back,
// dropping the oldest threads from the store as well so the screen and the
// file agree. It logs how many, never what they said.
func (model *Model) fitLocalThreads(key localthreads.Key) tea.Cmd {
	dropped := model.localThreads.Fit(key)
	if dropped == 0 {
		return nil
	}
	model.log.Warn("local threads dropped to keep the file small", "dropped", dropped, "number", key.Number)
	return model.broadcast(LocalThreadsMessage{Threads: model.localThreads.Snapshot()})
}

// writeLocalThreads starts a write of the threads as they are now.
func (model *Model) writeLocalThreads(key localthreads.Key) tea.Cmd {
	files := &model.localFiles
	path, generation, ok := files.BeginWrite(key)
	if !ok {
		return nil
	}
	threads := localthreads.ToFile(model.localThreads.ForRequest(key))
	return func() tea.Msg {
		return localThreadsSavedMessage{key: key, err: files.Write(key, path, generation, threads)}
	}
}

// localThreadsSaved ends a write. A change that came meanwhile is written now.
func (model *Model) localThreadsSaved(message localThreadsSavedMessage) tea.Cmd {
	if !model.localFiles.EndWrite(message.key, message.err) {
		return nil
	}
	return model.writeLocalThreads(message.key)
}
