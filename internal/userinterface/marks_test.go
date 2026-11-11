package userinterface

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/pullrequestmarks"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func marksKey(number int) pullrequestmarks.Key {
	return pullrequestmarks.Key{Host: "github.com", Repository: github.Repository{Owner: "acme", Name: "widgets"}, Number: number}
}

// quietTick never fires, so a status message stays until the next one.
func quietTick(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

// completingView is a view with a command whose arguments it completes.
type completingView struct {
	*listerView
	arguments []string
}

func (view *completingView) Update(message tea.Msg) (View, tea.Cmd) {
	_, command := view.listerView.Update(message)
	return view, command
}

func (view *completingView) CompleteArguments(name string) []string {
	if name != "mark" {
		return nil
	}
	return view.arguments
}

type marksSession struct {
	test      *testing.T
	model     *Model
	root      *fakeView
	directory string
}

func newMarksSession(test *testing.T, options Options) *marksSession {
	test.Helper()
	session := &marksSession{test: test, root: newFake("root"), directory: options.StateDirectory}
	options.Tick = quietTick
	session.model = New(session.root, theme.Default(), options)
	session.model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	return session
}

// settle runs a command and feeds every message it produces back to the model.
func (session *marksSession) settle(command tea.Cmd) {
	session.test.Helper()
	if command == nil {
		return
	}
	switch message := command().(type) {
	case nil:
	case tea.BatchMsg:
		for _, inner := range message {
			session.settle(inner)
		}
	default:
		// tea.Sequence's message is an unexported slice of commands; run them in order.
		if commands, ok := reflect.ValueOf(message).Interface().([]tea.Cmd); ok || reflect.TypeOf(message).ConvertibleTo(reflect.TypeOf([]tea.Cmd(nil))) {
			if !ok {
				commands = reflect.ValueOf(message).Convert(reflect.TypeOf(commands)).Interface().([]tea.Cmd)
			}
			for _, inner := range commands {
				session.settle(inner)
			}
			return
		}
		_, next := session.model.Update(message)
		session.settle(next)
	}
}

func (session *marksSession) send(message tea.Msg) {
	session.test.Helper()
	_, command := session.model.Update(message)
	session.settle(command)
}

func (session *marksSession) status() (string, statusKind) {
	return session.model.status.text, session.model.status.kind
}

// sent is the last MarksMessage a view was sent.
func lastMarks(view *fakeView) (pullrequestmarks.Marks, bool) {
	for i := len(view.got) - 1; i >= 0; i-- {
		if message, ok := view.got[i].(MarksMessage); ok {
			return message.Marks, true
		}
	}
	return pullrequestmarks.Marks{}, false
}

func savedMarks(test *testing.T, directory string) pullrequestmarks.Marks {
	test.Helper()
	marks, err := pullrequestmarks.Read(pullrequestmarks.Path(directory))
	if err != nil {
		test.Fatal(err)
	}
	return marks
}

func TestSetMarkSendsSavesAndReports(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})

	session.send(SetMarkMessage{Key: marksKey(12), Text: "  waiting on author "})

	if text, kind := session.status(); text != "Marked #12: waiting on author" || kind != statusInfo {
		test.Errorf("status = %q (%v)", text, kind)
	}
	for name, marks := range map[string]pullrequestmarks.Marks{"sent to the view": mustLastMarks(test, session.root), "saved": savedMarks(test, directory)} {
		if text, ok := marks.Mark(marksKey(12)); !ok || text != "waiting on author" {
			test.Errorf("%s: mark = %q, %v", name, text, ok)
		}
	}

	session.send(ClearMarkMessage{Key: marksKey(12)})
	if text, kind := session.status(); text != "Unmarked #12" || kind != statusInfo {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if _, ok := mustLastMarks(test, session.root).Mark(marksKey(12)); ok {
		test.Error("the view still has the mark")
	}
	if _, ok := savedMarks(test, directory).Mark(marksKey(12)); ok {
		test.Error("the file still has the mark")
	}
}

func mustLastMarks(test *testing.T, view *fakeView) pullrequestmarks.Marks {
	test.Helper()
	marks, ok := lastMarks(view)
	if !ok {
		test.Fatal("the view was sent no marks")
	}
	return marks
}

func TestMarkRefusedByTheStore(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})

	session.send(SetMarkMessage{Key: marksKey(12), Text: strings.Repeat("x", pullrequestmarks.MaxLength+1)})

	text, kind := session.status()
	if want := "not marked: mark is too long (at most 40 characters)"; text != want || kind != statusError {
		test.Errorf("status = %q (%v), want %q", text, kind, want)
	}
	if _, ok := lastMarks(session.root); ok {
		test.Error("the view was told of a refused mark")
	}
	if _, err := os.Stat(pullrequestmarks.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("a file was written: %v", err)
	}
}

func TestMarksStartWithTheLoadedOnesAndReachEveryView(test *testing.T) {
	loaded, err := pullrequestmarks.Marks{}.Set(marksKey(3), "left for Sam")
	if err != nil {
		test.Fatal(err)
	}
	session := newMarksSession(test, Options{Marks: loaded})
	if marks := mustLastMarks(test, session.root); marks.Names()[0] != "left for Sam" {
		test.Errorf("root view marks = %v", marks.Names())
	}
	pushed := newFake("pushed")
	session.send(PushMessage{View: pushed})
	if marks := mustLastMarks(test, pushed); marks.Names()[0] != "left for Sam" {
		test.Errorf("pushed view marks = %v", marks.Names())
	}
}

func TestNoMarksMeansNoMessageOnPush(test *testing.T) {
	session := newMarksSession(test, Options{})
	pushed := newFake("pushed")
	session.send(PushMessage{View: pushed})
	if _, ok := lastMarks(pushed); ok || len(session.root.got) != 1 { // only the size
		test.Errorf("marks sent with none set: root got %#v", session.root.got)
	}
}

func TestMarksWithoutAStateDirectoryLastForTheSession(test *testing.T) {
	session := newMarksSession(test, Options{})
	session.send(SetMarkMessage{Key: marksKey(1), Text: "wip"})
	if text, kind := session.status(); text != "Marked #1: wip" || kind != statusInfo {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if marks := mustLastMarks(test, session.root); len(marks.Names()) != 1 {
		test.Errorf("marks = %v", marks.Names())
	}
}

func TestUnreadableFileIsShownOnceAndNeverWritten(test *testing.T) {
	directory := test.TempDir()
	path := pullrequestmarks.Path(directory)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		test.Fatal(err)
	}
	marks, readErr := pullrequestmarks.Read(path)
	if readErr == nil {
		test.Fatal("the corrupt file was read")
	}
	session := newMarksSession(test, Options{StateDirectory: directory, Marks: marks, MarksError: readErr})

	session.settle(session.model.Init())
	text, kind := session.status()
	if kind != statusError || !strings.Contains(text, "marks file can't be decoded") || !strings.HasSuffix(text, "marks are not saved this session") {
		test.Errorf("startup status = %q (%v)", text, kind)
	}

	session.send(SetMarkMessage{Key: marksKey(12), Text: "wip"})
	text, kind = session.status()
	if want := "Marked #12 (not saved: the marks file could not be read at startup)"; text != want || kind != statusError {
		test.Errorf("status = %q (%v), want %q", text, kind, want)
	}
	if _, ok := mustLastMarks(test, session.root).Mark(marksKey(12)); !ok {
		test.Error("the mark was not kept for the session")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "not json" {
		test.Errorf("the unreadable file was changed: %q, %v", data, err)
	}
}

func TestFailedSaveIsShown(test *testing.T) {
	// A file where the directory should be makes the write fail.
	directory := filepath.Join(test.TempDir(), "state")
	if err := os.WriteFile(directory, nil, 0o600); err != nil {
		test.Fatal(err)
	}
	session := newMarksSession(test, Options{StateDirectory: directory})
	session.send(SetMarkMessage{Key: marksKey(12), Text: "wip"})
	if text, kind := session.status(); !strings.HasPrefix(text, "marks not saved: ") || kind != statusError {
		test.Errorf("status = %q (%v)", text, kind)
	}
}

func TestChangeDuringASaveQueuesOneFollowUp(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})
	part := &session.model.marks

	// The first save is started but its command is held back, so it is in flight.
	_, first := session.model.Update(SetMarkMessage{Key: marksKey(1), Text: "first"})
	if !part.saving {
		test.Fatal("no save in flight")
	}
	var held []tea.Cmd
	for _, command := range []tea.Msg{SetMarkMessage{Key: marksKey(2), Text: "second"}, SetMarkMessage{Key: marksKey(3), Text: "third"}} {
		_, next := session.model.Update(command)
		held = append(held, next)
	}
	if !part.pending {
		test.Error("changes during a save did not set pending")
	}
	// None of the later changes started a save of its own.
	for _, command := range held {
		for _, message := range flattenMessages(command) {
			if _, ok := message.(marksSavedMessage); ok {
				test.Fatal("a second save was started while one was in flight")
			}
		}
	}
	if _, err := os.Stat(pullrequestmarks.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Fatalf("the file exists before the first save ran: %v", err)
	}

	// The first save ends; exactly one follow-up carries the latest marks.
	var saves []marksSavedMessage
	for _, message := range flattenMessages(first) {
		if saved, ok := message.(marksSavedMessage); ok {
			saves = append(saves, saved)
		}
	}
	if len(saves) != 1 {
		test.Fatalf("first command produced %d save results", len(saves))
	}
	_, followUp := session.model.Update(saves[0])
	if part.pending || !part.saving {
		test.Errorf("after the first result: pending %v, saving %v", part.pending, part.saving)
	}
	var followUps []marksSavedMessage
	for _, message := range flattenMessages(followUp) {
		if saved, ok := message.(marksSavedMessage); ok {
			followUps = append(followUps, saved)
		}
	}
	if len(followUps) != 1 {
		test.Fatalf("%d follow-up saves, want 1", len(followUps))
	}
	if names := savedMarks(test, directory).Names(); len(names) != 3 {
		test.Errorf("saved marks = %v, want all three", names)
	}
	session.send(followUps[0])
	if part.saving || part.pending {
		test.Error("still saving after the follow-up ended")
	}
}

// flattenMessages runs a command and returns what it produced, batches opened.
func flattenMessages(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	switch message := command().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var all []tea.Msg
		for _, inner := range message {
			all = append(all, flattenMessages(inner)...)
		}
		return all
	default:
		return []tea.Msg{message}
	}
}

func TestSaveFailureIsShownAndTheNextSaveStillRuns(test *testing.T) {
	session := newMarksSession(test, Options{StateDirectory: test.TempDir()})
	session.send(SetMarkMessage{Key: marksKey(1), Text: "first"})
	session.send(marksSavedMessage{err: errors.New("disk full")})
	if text, kind := session.status(); text != "marks not saved: disk full" || kind != statusError {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if session.model.marks.saving {
		test.Error("still saving after the result")
	}
}

func TestMarkTextStaysOutOfTheLog(test *testing.T) {
	directory := test.TempDir()
	var logs bytes.Buffer
	unreadable := errors.New("marks file can't be decoded")
	session := newMarksSession(test, Options{StateDirectory: directory, MarksError: unreadable, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	session.send(SetMarkMessage{Key: marksKey(12), Text: "secret-note"})
	if text, _ := session.status(); strings.Contains(text, "secret-note") {
		test.Errorf("status holds the mark: %q", text)
	}
	if strings.Contains(logs.String(), "secret-note") {
		test.Errorf("log holds the mark: %s", logs.String())
	}
}

func TestViewsBelowTheTopHearOfChanges(test *testing.T) {
	session := newMarksSession(test, Options{})
	session.send(PushMessage{View: newFake("top")})
	session.send(SetMarkMessage{Key: marksKey(12), Text: "wip"})
	if _, ok := mustLastMarks(test, session.root).Mark(marksKey(12)); !ok {
		test.Error("the view below the top was not sent the new mark")
	}
	session.send(ClearMarkMessage{Key: marksKey(12)})
	if _, ok := mustLastMarks(test, session.root).Mark(marksKey(12)); ok {
		test.Error("the view below the top was not sent the cleared marks")
	}
}

func TestStartupNoticesKeepTheMarksErrorLast(test *testing.T) {
	session := newMarksSession(test, Options{Notice: "no remote", MarksError: errors.New("marks file can't be decoded")})
	session.settle(session.model.Init())
	if text, kind := session.status(); kind != statusError || !strings.Contains(text, "decoded") {
		test.Errorf("status = %q (%v)", text, kind)
	}
}

func TestMarkCompletionCompletesTheWholeArgument(test *testing.T) {
	names := []string{"left for Sam", "waiting on author", "waiting on CI"}
	newModel := func() *Model {
		return newCompletionModel(&completingView{listerView: newLister("top", "mark", "unmark"), arguments: names})
	}
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"one match", []string{"mark l", "tab"}, "mark left for Sam"},
		{"a prefix with a space", []string{"mark waiting o", "tab"}, "mark waiting on author"},
		{"next match", []string{"mark waiting on", "tab", "tab"}, "mark waiting on CI"},
		{"no argument yet", []string{"mark", "space", "tab"}, "mark left for Sam"},
		{"nothing matches", []string{"mark zzz", "tab"}, "mark zzz"},
		{"a command with no completions", []string{"unmark ", "tab"}, "unmark "},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := newModel()
			typeKeys(model, testCase.keys...)
			if got := model.commandLine.input.Value(); got != testCase.want {
				test.Errorf("line = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestMarkCompletionAsksOnlyForTheViewsOwnCommands(test *testing.T) {
	model := newCompletionModel(&completingView{listerView: newLister("top", "unmark"), arguments: []string{"wip"}})
	typeKeys(model, "mark w", "tab")
	if got := model.commandLine.input.Value(); got != "mark w" {
		test.Errorf("line = %q", got)
	}
}

func TestMarkKeyAndClientHost(test *testing.T) {
	pullRequest := github.PullRequest{Repository: github.Repository{Owner: "acme", Name: "widgets"}, Number: 12}
	want := pullrequestmarks.Key{Host: "ghe.example.test", Repository: pullRequest.Repository, Number: 12}
	if diff := cmp.Diff(want, MarkKey("ghe.example.test", pullRequest)); diff != "" {
		test.Errorf("key (-want +got):\n%s", diff)
	}
	if got := ClientHost(nil); got != "" {
		test.Errorf("host of no client = %q", got)
	}
}
