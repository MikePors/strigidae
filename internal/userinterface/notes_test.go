package userinterface

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/pullrequestnotes"
)

func notesKey(number int) pullrequestnotes.Key {
	return pullrequestnotes.Key{Host: "github.com", Repository: github.Repository{Owner: "acme", Name: "widgets"}, Number: number}
}

func lastNotes(view *fakeView) (pullrequestnotes.Notes, bool) {
	for i := len(view.got) - 1; i >= 0; i-- {
		if message, ok := view.got[i].(NotesMessage); ok {
			return message.Notes, true
		}
	}
	return pullrequestnotes.Notes{}, false
}

func savedNotes(test *testing.T, directory string) pullrequestnotes.Notes {
	test.Helper()
	notes, err := pullrequestnotes.Read(pullrequestnotes.Path(directory))
	if err != nil {
		test.Fatal(err)
	}
	return notes
}

func TestSetNoteSendsSavesAndReports(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})

	session.send(SetNoteMessage{Key: notesKey(12), Text: "ask Sam about the cache\nthen re-check tests"})

	text, kind := session.status()
	if want := "Saved the note on #12"; text != want || kind != statusInfo {
		test.Errorf("status = %q (%v), want %q", text, kind, want)
	}
	sent, ok := lastNotes(session.root)
	if !ok {
		test.Fatal("the view was sent no notes")
	}
	for name, notes := range map[string]pullrequestnotes.Notes{"sent to the view": sent, "saved": savedNotes(test, directory)} {
		if note, ok := notes.Note(notesKey(12)); !ok || note != "ask Sam about the cache\nthen re-check tests" {
			test.Errorf("%s: note = %q, %v", name, note, ok)
		}
	}

	session.send(ClearNoteMessage{Key: notesKey(12)})
	if text, kind := session.status(); text != "Deleted the note on #12" || kind != statusInfo {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if _, ok := savedNotes(test, directory).Note(notesKey(12)); ok {
		test.Error("the file still has the note")
	}
}

func TestRefusedNoteChangesNothing(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})

	session.send(SetNoteMessage{Key: notesKey(12), Text: " \n "})

	if text, kind := session.status(); text != "note not saved: note is empty" || kind != statusError {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if _, ok := lastNotes(session.root); ok {
		test.Error("the view was told of a refused note")
	}
	if _, err := os.Stat(pullrequestnotes.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("a file was written: %v", err)
	}
}

func TestNotesStartWithTheLoadedOnesAndReachEveryView(test *testing.T) {
	loaded, err := pullrequestnotes.Notes{}.Set(notesKey(3), "from last week")
	if err != nil {
		test.Fatal(err)
	}
	session := newMarksSession(test, Options{Notes: loaded})
	if notes, _ := lastNotes(session.root); notes.Empty() {
		test.Error("the root view was not sent the loaded notes")
	}
	pushed := newFake("pushed")
	session.send(PushMessage{View: pushed})
	if notes, _ := lastNotes(pushed); notes.Empty() {
		test.Error("the pushed view was not sent the notes")
	}
}

func TestNoNotesMeansNoMessageOnPush(test *testing.T) {
	session := newMarksSession(test, Options{})
	pushed := newFake("pushed")
	session.send(PushMessage{View: pushed})
	if _, ok := lastNotes(pushed); ok {
		test.Error("notes sent with none set")
	}
}

func TestNotesWithoutAStateDirectoryLastForTheSession(test *testing.T) {
	session := newMarksSession(test, Options{})
	session.send(SetNoteMessage{Key: notesKey(12), Text: "wip"})
	if text, kind := session.status(); text != "Saved the note on #12" || kind != statusInfo {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if notes, _ := lastNotes(session.root); notes.Empty() {
		test.Error("the note was not kept for the session")
	}
}

func TestUnreadableNotesFileIsShownOnceAndNeverWritten(test *testing.T) {
	directory := test.TempDir()
	path := pullrequestnotes.Path(directory)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		test.Fatal(err)
	}
	notes, readErr := pullrequestnotes.Read(path)
	if readErr == nil {
		test.Fatal("the corrupt file was read")
	}
	session := newMarksSession(test, Options{StateDirectory: directory, Notes: notes, NotesError: readErr})

	session.settle(session.model.Init())
	if text, kind := session.status(); kind != statusError || !strings.HasSuffix(text, "notes are not saved this session") {
		test.Errorf("startup status = %q (%v)", text, kind)
	}

	session.send(SetNoteMessage{Key: notesKey(12), Text: "wip"})
	if text, kind := session.status(); kind != statusError || !strings.Contains(text, "not kept") {
		test.Errorf("status = %q (%v)", text, kind)
	}
	if notes, _ := lastNotes(session.root); notes.Empty() {
		test.Error("the note was not kept for the session")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "not json" {
		test.Errorf("the unreadable file was changed: %q, %v", data, err)
	}
}

func TestFailedNoteSaveIsShownWithoutTheText(test *testing.T) {
	// A file where the directory should be makes the write fail.
	directory := filepath.Join(test.TempDir(), "state")
	if err := os.WriteFile(directory, nil, 0o600); err != nil {
		test.Fatal(err)
	}
	session := newMarksSession(test, Options{StateDirectory: directory})
	session.send(SetNoteMessage{Key: notesKey(12), Text: "private words"})
	text, kind := session.status()
	if !strings.HasPrefix(text, "notes not saved: ") || kind != statusError || strings.Contains(text, "private words") {
		test.Errorf("status = %q (%v)", text, kind)
	}
}

func TestNoteChangedDuringASaveIsSavedAfterwards(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})
	part := &session.model.notes
	part.saving = true // a save is in flight

	session.send(SetNoteMessage{Key: notesKey(12), Text: "second"})
	if !part.pending {
		test.Fatal("a change during a save was not queued")
	}
	if _, err := os.Stat(pullrequestnotes.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Fatalf("a second save started while one was in flight: %v", err)
	}

	session.send(notesSavedMessage{})
	if part.pending {
		test.Error("the queued save was not started")
	}
	if note, _ := savedNotes(test, directory).Note(notesKey(12)); note != "second" {
		test.Errorf("saved note = %q, want the latest", note)
	}
}

func TestSetNoteCommandNeedsAHost(test *testing.T) {
	key := notesKey(12)
	key.Host = ""
	message := SetNoteCommand(key, "text")()
	failure, ok := message.(ErrorMessage)
	if !ok || !strings.Contains(failure.Err.Error(), "host") {
		test.Errorf("message = %#v, want an error about the host", message)
	}
	if _, ok := SetNoteCommand(notesKey(12), "text")().(SetNoteMessage); !ok {
		test.Error("a note with a host was not sent on")
	}
}

func TestFlushAtQuitWritesTheLatestNotesOnce(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})
	part := &session.model.notes
	part.saving = true // a save is in flight, so nothing reaches the disk by itself

	session.send(SetNoteMessage{Key: notesKey(12), Text: "written only at quit"})
	if _, err := os.Stat(pullrequestnotes.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Fatalf("the file exists before the flush: %v", err)
	}
	<-part.flush(session.model.log)
	if note, _ := savedNotes(test, directory).Note(notesKey(12)); note != "written only at quit" {
		test.Errorf("saved note = %q", note)
	}

	// The save that was in flight finishes after the flush with an older copy: it must not win.
	older, _ := pullrequestnotes.Encode(pullrequestnotes.Notes{})
	if err := part.saver.Save(part.generation-1, older); err != nil {
		test.Fatal(err)
	}
	if note, _ := savedNotes(test, directory).Note(notesKey(12)); note != "written only at quit" {
		test.Errorf("an older copy replaced the flushed notes: %q", note)
	}
}

func TestFlushAtQuitHasNothingToDoWithoutChangesOrWithAnUnreadableFile(test *testing.T) {
	directory := test.TempDir()
	session := newMarksSession(test, Options{StateDirectory: directory})
	<-session.model.notes.flush(session.model.log)
	if _, err := os.Stat(pullrequestnotes.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("a flush with no changes wrote a file: %v", err)
	}

	session = newMarksSession(test, Options{StateDirectory: directory, NotesError: errors.New("unreadable")})
	session.send(SetNoteMessage{Key: notesKey(12), Text: "kept for the session"})
	<-session.model.notes.flush(session.model.log)
	if _, err := os.Stat(pullrequestnotes.Path(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("a flush overwrote an unreadable file: %v", err)
	}
}
