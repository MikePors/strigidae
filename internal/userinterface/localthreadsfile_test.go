package userinterface

import (
	"os"
	"path/filepath"
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
	// Force the flush to outlast Close too, as a slow disk can on CI.
	release := session.model.localFiles.HoldForTest()
	defer release()
	session.model.Close()
	close(runner.let)
	release()
	select {
	case <-session.model.localFiles.Flushed():
	case <-time.After(10 * time.Second):
		test.Fatal("local thread flush did not finish")
	}
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
	release := files.HoldForTest()
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

// A question that was waiting when strig exited has no answer now, and that is
// saved back to the file.
func TestRestartWhileWaitingIsSavedBack(test *testing.T) {
	session := newSavedSession(test, savedOptions{directory: test.TempDir(), host: "github.com"})
	data, err := localthreads.Encode([]localthreads.Thread{{Identifier: "local-1", Question: "why?", State: localthreads.StateWaiting, Asked: time.Unix(1, 0)}})
	if err != nil {
		test.Fatal(err)
	}
	if err := localthreads.Write(session.path(), data); err != nil {
		test.Fatal(err)
	}
	session.open()
	got := session.shown()
	if len(got) != 1 || got[0].State != github.LocalFailed || got[0].Text != "strig was closed" {
		test.Fatalf("after restart = %+v", got)
	}
	saved := readSaved(test, session.path())
	if len(saved) != 1 || saved[0].State != localthreads.StateFailed || saved[0].Text != "strig was closed" {
		test.Errorf("file after restart = %+v", saved)
	}
}

// A write command is held back and run by hand, to see what a change made
// while it was in flight does.
func TestWritesAreOneAtATimeAndInOrder(test *testing.T) {
	session := newSavedSession(test, savedOptions{directory: test.TempDir(), host: "github.com"})
	session.open()
	identity, _ := session.view.PullRequestIdentity()
	key := localthreads.Key{Host: "github.com", Repository: identity.Repository, Number: identity.Number}
	ask := func(question string) {
		session.model.localThreads.Add(LocalThread{Host: "github.com", Repository: identity.Repository, Number: identity.Number, Question: question, Asked: session.clock})
	}
	saved := func() []string { return questions(fromSaved(readSaved(test, session.path()))) }

	ask("first")
	firstWrite := session.model.saveLocalThreads(key)
	if firstWrite == nil {
		test.Fatal("a change with nothing in flight did not start a write")
	}
	ask("second")
	if session.model.saveLocalThreads(key) != nil {
		test.Error("a second write started while the first was in flight")
	}
	ask("third")
	if session.model.saveLocalThreads(key) != nil {
		test.Error("a third write started while the first was in flight")
	}
	// The first write holds only what was there when it started.
	first := firstWrite().(localThreadsSavedMessage)
	if diff := cmp.Diff([]string{"first"}, saved()); first.err != nil || diff != "" {
		test.Fatalf("file after the first write (%v) (-want +got):\n%s", first.err, diff)
	}
	// Its end starts one write of the latest copy.
	latest := session.model.localThreadsSaved(first)
	if latest == nil {
		test.Fatal("no write followed for the changes made meanwhile")
	}
	last := latest().(localThreadsSavedMessage)
	if diff := cmp.Diff([]string{"first", "second", "third"}, saved()); last.err != nil || diff != "" {
		test.Errorf("file after the last write (%v) (-want +got):\n%s", last.err, diff)
	}
	if session.model.localThreadsSaved(last) != nil {
		test.Error("a write followed with nothing changed")
	}
	ask("fourth") // idle again: the next change writes at once
	if session.model.saveLocalThreads(key) == nil {
		test.Error("a change after the writes ended did not write")
	}
}

func fromSaved(saved []localthreads.Thread) []LocalThread {
	return localthreads.FromFile(localthreads.Key{}, saved)
}

func TestNothingIsKeptWithoutAUsableHost(test *testing.T) {
	for _, testCase := range []struct {
		name       string
		host       string
		repository github.Repository
	}{
		{"empty host", "", widgetsRepository},
		{"slash in host", "a/b", widgetsRepository},
		{"dot dot owner", "github.com", github.Repository{Owner: "..", Name: "widgets"}},
		{"slash in name", "github.com", github.Repository{Owner: "acme", Name: "x/y"}},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			directory := test.TempDir()
			session := newSavedSession(test, savedOptions{directory: directory, host: testCase.host, repository: testCase.repository})
			session.open()
			session.ask("why?")
			session.ask("and?")
			session.model.Close()
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
				test.Errorf("state directory holds %v (%v), want nothing", entries, err)
			}
			if got := len(session.shown()); got != 2 {
				test.Errorf("the session shows %d threads, want 2: they still work", got)
			}
			if got := strings.Count(session.logs.String(), "local threads are not kept"); got != 1 {
				test.Errorf("explained %d times, want once:\n%s", got, session.logs)
			}
		})
	}
}

func TestNoStateDirectoryKeepsNothingAndSaysNothing(test *testing.T) {
	session := newSavedSession(test, savedOptions{host: "github.com"})
	session.open()
	session.ask("why?")
	session.model.Close()
	if got := len(session.shown()); got != 1 {
		test.Errorf("shows %d threads, want 1", got)
	}
	if session.logs.Len() != 0 {
		test.Errorf("logged: %s", session.logs)
	}
}

func TestFailedWriteIsLoggedWithoutText(test *testing.T) {
	directory := test.TempDir()
	session := newSavedSession(test, savedOptions{directory: directory, host: "github.com"})
	session.open()
	// A file where the directory should be makes every write fail.
	if err := os.WriteFile(filepath.Join(directory, "strig"), []byte("x"), 0o600); err != nil {
		test.Fatal(err)
	}
	session.ask("top secret question")
	if !strings.Contains(session.logs.String(), "local threads not saved") {
		test.Errorf("failure not logged: %s", session.logs)
	}
	if strings.Contains(session.logs.String(), "secret") {
		test.Errorf("log holds thread text: %s", session.logs)
	}
	if got := len(session.shown()); got != 1 {
		test.Errorf("the thread is not shown after a failed write: %d", got)
	}
}

// An unusable file found while another pull request's write is stuck must not
// block the update goroutine, and is still set aside before the next write.
func TestFailedReadDoesNotWaitForAStuckWrite(test *testing.T) {
	session := newSavedSession(test, savedOptions{directory: test.TempDir(), host: "github.com"})
	if err := os.MkdirAll(filepath.Dir(session.path()), 0o700); err != nil {
		test.Fatal(err)
	}
	content := []byte("{secret-not-json")
	if err := os.WriteFile(session.path(), content, 0o600); err != nil {
		test.Fatal(err)
	}
	identity, _ := session.view.PullRequestIdentity()
	key := localthreads.Key{Host: "github.com", Repository: identity.Repository, Number: identity.Number}
	read := session.model.update(PullRequestOpenedMessage{Identity: identity})
	loaded, ok := read().(localThreadsLoadedMessage)
	if !ok || loaded.err == nil {
		test.Fatalf("read result = %+v, want a failed read", loaded)
	}

	files := &session.model.localFiles
	release := files.HoldForTest()
	defer release()
	var handled tea.Cmd
	done := make(chan struct{})
	go func() {
		handled = session.model.update(loaded)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		test.Fatal("handling a failed read waited for the write lock")
	}
	release()
	session.run(handled)
	if strings.Contains(session.logs.String(), "secret") {
		test.Errorf("log holds file content: %s", session.logs)
	}

	session.model.localThreads.Add(LocalThread{Host: "github.com", Repository: identity.Repository, Number: identity.Number, Question: "why?", Asked: session.clock})
	session.run(session.model.saveLocalThreads(key))
	aside, err := os.ReadFile(session.path() + localthreads.UnreadableSuffix)
	if err != nil || string(aside) != string(content) {
		test.Errorf("unreadable copy missing or changed: %v", err)
	}
	if saved := readSaved(test, session.path()); len(saved) != 1 || saved[0].Question != "why?" {
		test.Errorf("new file = %+v", saved)
	}
}
