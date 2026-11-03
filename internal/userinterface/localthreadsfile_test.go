package userinterface

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/localthreads"
	"github.com/MikePors/strigidae/internal/plugins"
)

func TestLocalThreadsSurviveARestart(test *testing.T) {
	directory := test.TempDir()
	first := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	first.open()
	first.ask("why?")
	first.ask("and this?")
	first.model.Close()

	second := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	second.open()
	got := second.shown()
	if diff := cmp.Diff([]string{"why?", "and this?"}, questions(got)); diff != "" {
		test.Fatalf("questions after restart (-want +got):\n%s", diff)
	}
	for _, thread := range got {
		if thread.State != github.LocalAnswered || thread.Text != "because" || thread.Head != "abc" || thread.Path != "a.go" || thread.Line != 5 {
			test.Errorf("thread = %+v", thread)
		}
		if thread.Repository != widgetsRepository || thread.Number != 7 {
			test.Errorf("thread belongs to %v #%d", thread.Repository, thread.Number)
		}
	}
}

// A question asked before the saved threads have been read must not overwrite
// them: its write waits for the read, and the file ends up with both.
func TestAskingBeforeTheLoadEndsKeepsTheSavedThreads(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	data, err := localthreads.Encode([]localthreads.Thread{{Identifier: "local-1", Question: "old", State: localthreads.StateAnswered, Asked: time.Unix(1, 0)}})
	if err != nil {
		test.Fatal(err)
	}
	if err := localthreads.Write(session.path(), data); err != nil {
		test.Fatal(err)
	}
	identity, _ := session.view.PullRequestIdentity()
	read := session.model.update(PullRequestOpenedMessage{Identity: identity}) // held back
	session.ask("new")
	session.run(read)
	var got []string
	for _, thread := range readSaved(test, session.path()) {
		got = append(got, thread.Question)
	}
	if diff := cmp.Diff([]string{"old", "new"}, got); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"old", "new"}, questions(session.shown())); diff != "" {
		test.Errorf("shown (-want +got):\n%s", diff)
	}
}

// The order of events in a real run: the question is asked before the root has
// heard that the pull request opened. The saved thread must survive.
func TestAskingBeforeOpeningKeepsTheSavedThreads(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	data, err := localthreads.Encode([]localthreads.Thread{{Identifier: "local-1", Question: "old", State: localthreads.StateAnswered, Asked: time.Unix(1, 0)}})
	if err != nil {
		test.Fatal(err)
	}
	if err := localthreads.Write(session.path(), data); err != nil {
		test.Fatal(err)
	}
	session.ask("new")
	session.open()
	var got []string
	for _, thread := range readSaved(test, session.path()) {
		got = append(got, thread.Question)
	}
	if diff := cmp.Diff([]string{"old", "new"}, got); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"old", "new"}, questions(session.shown())); diff != "" {
		test.Errorf("shown (-want +got):\n%s", diff)
	}
}

// Close makes good a write command that was never run, and does not wait for
// it: the program ended before Bubble Tea ran the batch.
func TestCloseWritesWhenTheWriteCommandNeverRan(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	session.open()
	session.model.update(session.request("lost")) // its commands are dropped
	session.model.Close()
	if saved := readSaved(test, session.path()); len(saved) != 1 || saved[0].Question != "lost" {
		test.Errorf("file after Close = %+v", saved)
	}
}

// The same when a plugin run outlasts the quit wait.
func TestCloseWritesWhenARunOutlastsTheWait(test *testing.T) {
	directory := test.TempDir()
	runner := &stubbornRunner{started: make(chan struct{}), let: make(chan struct{})}
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com", runner: runner, quitWait: 50 * time.Millisecond})
	session.open()
	results := make(chan tea.Msg, 4)
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
			results <- message
		}()
	}
	spawn(session.model.update(session.request("stuck")))
	<-runner.started
	session.model.update(session.request("lost")) // its commands are dropped
	session.model.Close()
	close(runner.let)
	saved := readSaved(test, session.path())
	if len(saved) != 2 {
		test.Errorf("file after Close = %+v, want both questions", saved)
	}
}

// bigAnswer is an answer of about size bytes of ordinary text.
func bigAnswer(size int) string { return strings.Repeat("0123456789abcdef", size/16) }

// Five answers of about 900 KB each are more than the file may hold. The file
// is trimmed, oldest first, so it can be read back, and the screen agrees.
func TestSavedFileAlwaysFitsWhatCanBeReadBack(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	session.runner = &fakeRunner{effects: []plugins.Effect{{Name: plugins.EffectAnswer, Text: bigAnswer(900_000)}}}
	session.model.pluginRunner = session.runner
	session.open()
	var asked []string
	for _, question := range []string{"q1", "q2", "q3", "q4", "q5"} {
		session.ask(question)
		asked = append(asked, question)
	}
	file, err := localthreads.Read(session.path())
	if err != nil {
		test.Fatalf("the saved file can't be read back: %v", err)
	}
	var kept []string
	for _, thread := range file.Threads {
		kept = append(kept, thread.Question)
	}
	if len(kept) == 0 || len(kept) >= len(asked) || kept[len(kept)-1] != "q5" {
		test.Fatalf("kept %v of %v, want the newest only, fewer than all", kept, asked)
	}
	if diff := cmp.Diff(asked[len(asked)-len(kept):], kept); diff != "" {
		test.Errorf("kept threads are not the newest (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(kept, questions(session.shown())); diff != "" {
		test.Errorf("the screen and the file disagree (-file +screen):\n%s", diff)
	}
	if !strings.Contains(session.logs.String(), "dropped") || strings.Contains(session.logs.String(), "0123456789") {
		test.Errorf("log should count the dropped threads and hold no text:\n%.300s", session.logs)
	}
}

// A save that is stuck holds the file mutex. Quitting must still cancel the
// plugin runs and return within the quit wait.
func TestCloseIsBoundedWhileAWriteIsStuck(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com", quitWait: 100 * time.Millisecond})
	session.open()
	session.ask("first") // starts the plugin context
	session.model.update(session.request("lost"))
	files := &session.model.localFiles
	release := files.Hold()
	defer release()
	closed := make(chan struct{})
	go func() {
		session.model.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		test.Fatal("Close did not return while a write held the mutex")
	}
	if session.model.pluginContext.Err() == nil {
		test.Error("the plugin runs were not cancelled")
	}
	release()
	<-files.Flushed() // the stuck writes end before the directory is removed
}
