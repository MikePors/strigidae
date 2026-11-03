package userinterface

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/config"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/localthreads"
	"github.com/MikePors/strigidae/internal/plugins"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

var widgetsRepository = github.Repository{Owner: "acme", Name: "widgets"}

// savedSession is a root model over a pull request view, keeping local threads
// in a directory, with a clock that moves a minute each time it is read.
type savedSession struct {
	test   *testing.T
	model  *Model
	view   *pullRequestView
	logs   *bytes.Buffer
	runner PluginRunner
	clock  time.Time
}

type savedOptions struct {
	directory  string
	host       string
	limit      int
	repository github.Repository
	times      []time.Time  // asked times to use, in order, before the clock moves on
	runner     PluginRunner // replaces the one that answers at once
	quitWait   time.Duration
	bytes      int // the file size limit; 0 for the real one
}

func newSavedSession(test *testing.T, options savedOptions) *savedSession {
	test.Helper()
	session := &savedSession{
		test:   test,
		logs:   &bytes.Buffer{},
		runner: &fakeRunner{effects: []plugins.Effect{{Name: plugins.EffectAnswer, Text: "because"}}},
		clock:  time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC),
	}
	if options.repository.IsZero() {
		options.repository = widgetsRepository
	}
	if options.runner != nil {
		session.runner = options.runner
	}
	identity := PullRequestIdentity{Host: options.host, Repository: options.repository, Number: 7, Head: "abc"}
	session.view = newPullRequestView(identity)
	times := options.times
	quitWait := options.quitWait
	if quitWait == 0 {
		quitWait = 300 * time.Millisecond
	}
	session.model = New(session.view, theme.Default(), Options{
		Logger: slog.New(slog.NewTextHandler(session.logs, nil)),
		Tick: func(_ time.Duration, function func(time.Time) tea.Msg) tea.Cmd {
			return func() tea.Msg { return function(time.Time{}) }
		},
		ViewCommands:     testViewCommands,
		PluginRunner:     session.runner,
		PluginCommands:   []PluginCommand{{Name: "ask", Target: config.TargetLines, Show: config.ShowThread}},
		PluginQuitWait:   quitWait,
		StateDirectory:   options.directory,
		LocalThreadLimit: options.limit,
		LocalThreadBytes: options.bytes,
		Clock: func() time.Time {
			if len(times) > 0 {
				asked := times[0]
				times = times[1:]
				return asked
			}
			session.clock = session.clock.Add(time.Minute)
			return session.clock
		},
	})
	return session
}

// run performs a command and everything it leads to, in order, handing each
// message to the model.
func (session *savedSession) run(command tea.Cmd) {
	session.test.Helper()
	queue := []tea.Cmd{command}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 200 {
			session.test.Fatal("commands do not stop")
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		switch message := next().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, message...)
		default:
			queue = append(queue, session.model.update(message))
		}
	}
}

func (session *savedSession) open() {
	session.test.Helper()
	identity, _ := session.view.PullRequestIdentity()
	session.run(session.model.update(PullRequestOpenedMessage{Identity: identity}))
}

// request is what the view sends when :ask is run on a line.
func (session *savedSession) request(question string) RunPluginMessage {
	identity, _ := session.view.PullRequestIdentity()
	return RunPluginMessage{Name: "ask", Identity: identity, Request: plugins.Request{
		Command: "ask", Arguments: question, Head: identity.Head,
		Repository: plugins.Repository{Host: identity.Host, Owner: identity.Repository.Owner, Name: identity.Repository.Name},
		Number:     identity.Number,
		Target:     &plugins.Target{Path: "a.go", Side: "RIGHT", Line: 5, StartLine: 5},
	}}
}

// ask runs :ask on a line and delivers the answer.
func (session *savedSession) ask(question string) {
	session.test.Helper()
	session.run(session.model.update(session.request(question)))
}

// askHeld runs :ask on a line with a blockingRunner. It returns when the
// waiting thread has been written; the function it returns lets the plugin
// answer and delivers the answer.
func (session *savedSession) askHeld(question string) (answer func()) {
	session.test.Helper()
	messages := make(chan tea.Msg, 16)
	var spawn func(command tea.Cmd)
	spawn = func(command tea.Cmd) {
		if command == nil {
			return
		}
		go func() {
			message := command()
			if batch, ok := message.(tea.BatchMsg); ok {
				for _, member := range batch {
					spawn(member)
				}
				return
			}
			messages <- message
		}()
	}
	spawn(session.model.update(session.request(question)))
	for waiting := true; waiting; {
		select {
		case message := <-messages:
			if saved, ok := message.(localThreadsSavedMessage); ok {
				session.run(session.model.update(saved))
				waiting = false
			}
		case <-time.After(5 * time.Second):
			session.test.Fatal("asking did not write the waiting thread")
		}
	}
	return func() {
		session.test.Helper()
		close(session.runner.(*blockingRunner).release)
		for {
			if result, ok := (<-messages).(pluginResultMessage); ok {
				session.run(session.model.update(result))
				return
			}
		}
	}
}

// blockingRunner answers when released, or fails when its context ends.
type blockingRunner struct{ release chan struct{} }

func newBlockingRunner() *blockingRunner { return &blockingRunner{release: make(chan struct{})} }

func (runner *blockingRunner) Run(ctx context.Context, _ string, _ plugins.Request) ([]plugins.Effect, error) {
	select {
	case <-runner.release:
		return []plugins.Effect{{Name: plugins.EffectAnswer, Text: "late answer"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// shown is the local threads the view was last told of.
func (session *savedSession) shown() []LocalThread {
	var last []LocalThread
	for _, message := range session.view.got {
		if threads, ok := message.(LocalThreadsMessage); ok {
			last = threads.Threads
		}
	}
	return last
}

func (session *savedSession) path() string {
	session.test.Helper()
	identity, _ := session.view.PullRequestIdentity()
	return savedPath(session.test, session.model.localFiles.Directory, identity)
}

func savedPath(test *testing.T, directory string, identity PullRequestIdentity) string {
	test.Helper()
	path, err := localthreads.Path(directory, identity.Host, identity.Repository.Owner, identity.Repository.Name, identity.Number)
	if err != nil {
		test.Fatal(err)
	}
	return path
}

func readSaved(test *testing.T, path string) []localthreads.Thread {
	test.Helper()
	file, err := localthreads.Read(path)
	if err != nil {
		test.Fatal(err)
	}
	return file.Threads
}

func questions(threads []LocalThread) []string {
	var asked []string
	for _, thread := range threads {
		asked = append(asked, thread.Question)
	}
	return asked
}
