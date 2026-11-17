package workspace

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/pullrequestnotes"
	"github.com/MikePors/strigidae/internal/userinterface"
)

var noteKey = pullrequestnotes.Key{Host: "example.test", Repository: testRepository, Number: 7}

// noteComposer is a loaded workspace on the Overview with the editor stubbed.
func noteComposer(test *testing.T) *composer {
	test.Helper()
	testComposer := newComposer(test)
	testComposer.run(testComposer.view.Init())
	return testComposer
}

func (composer *composer) withNote(text string) {
	composer.test.Helper()
	notes, err := pullrequestnotes.Notes{}.Set(noteKey, text)
	if err != nil {
		composer.test.Fatal(err)
	}
	composer.send(userinterface.NotesMessage{Notes: notes})
}

// noteResult runs `:note` and returns what it asked of the root.
func (composer *composer) noteResult() []any {
	composer.test.Helper()
	command, handled := composer.view.Command("note", "")
	if !handled {
		composer.test.Fatal(":note is not handled")
	}
	var out []any
	for _, message := range messages(command) {
		if edited, ok := message.(noteTextMessage); ok {
			_, next := composer.view.Update(edited)
			for _, result := range messages(next) {
				out = append(out, result)
			}
			continue
		}
		out = append(out, message)
	}
	return out
}

func TestNoteSendsTheEditedText(test *testing.T) {
	composer := noteComposer(test)
	composer.body = "\n  first\r\nsecond  \n\n"
	got := composer.noteResult()
	want := []any{userinterface.SetNoteMessage{Key: noteKey, Text: "first\nsecond"}}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Errorf("results (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{""}, composer.openedOn); diff != "" {
		test.Errorf("the editor opened on (-want +got):\n%s", diff)
	}
}

func TestNoteOpensOnTheExistingNote(test *testing.T) {
	composer := noteComposer(test)
	composer.withNote("old note\nline two")
	composer.body = "old note\nline two\n"
	got := composer.noteResult()
	if diff := cmp.Diff([]string{"old note\nline two"}, composer.openedOn); diff != "" {
		test.Errorf("the editor opened on (-want +got):\n%s", diff)
	}
	if len(got) != 1 {
		test.Fatalf("results = %v, want one", got)
	}
	if info, ok := got[0].(userinterface.InfoMessage); !ok || info.Text != "no changes" {
		test.Errorf("an unchanged note gave %v, want the notice 'no changes'", got[0])
	}
}

func TestNoteRefusesWhatCannotBeKept(test *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		err  error
		want string
	}{
		{"empty text never deletes", "  \n", nil, "empty note"},
		{"the editor failed", "text", errors.New("exit status 1"), "edit note: exit status 1"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			composer := noteComposer(test)
			composer.withNote("keep me")
			composer.body, composer.err = testCase.body, testCase.err
			got := composer.noteResult()
			if len(got) != 1 {
				test.Fatalf("results = %v, want one", got)
			}
			failure, ok := got[0].(userinterface.ErrorMessage)
			if !ok || !strings.Contains(failure.Err.Error(), testCase.want) {
				test.Errorf("result = %v, want an error containing %q", got[0], testCase.want)
			}
		})
	}
}

func TestNoteNeedsTheLoadedPullRequest(test *testing.T) {
	harness := newHarness(test, 100, 24) // nothing has loaded yet
	opened := false
	harness.view.edit = func(string, func(string, error) tea.Msg) tea.Cmd { opened = true; return nil }
	command, handled := harness.view.Command("note", "")
	if !handled {
		test.Fatal(":note is not handled")
	}
	got := messages(command)
	if len(got) != 1 || opened {
		test.Fatalf("results = %v, editor opened = %v; want one notice and no editor", got, opened)
	}
	if info, ok := got[0].(userinterface.InfoMessage); !ok || info.Text != "pull request not loaded yet" {
		test.Errorf("result = %v", got[0])
	}
}

func TestNoteDeleteAsksThenClears(test *testing.T) {
	composer := noteComposer(test)
	composer.withNote("keep me")
	command, _ := composer.view.Command("note-delete", "")
	got := messages(command)
	if len(got) != 1 {
		test.Fatalf("results = %v, want a question", got)
	}
	question, ok := got[0].(userinterface.ConfirmMessage)
	if !ok || !strings.Contains(question.Text, "#7") {
		test.Fatalf("result = %v, want a question naming #7", got[0])
	}
	if diff := cmp.Diff([]tea.Msg{userinterface.ClearNoteMessage{Key: noteKey}}, messages(question.OnYes)); diff != "" {
		test.Errorf("the yes did (-want +got):\n%s", diff)
	}
}

func TestNoteDeleteWithoutANote(test *testing.T) {
	composer := noteComposer(test)
	command, _ := composer.view.Command("note-delete", "")
	got := messages(command)
	if len(got) != 1 {
		test.Fatalf("results = %v", got)
	}
	if info, ok := got[0].(userinterface.InfoMessage); !ok || info.Text != "#7 has no note" {
		test.Errorf("result = %v", got[0])
	}
}

func TestOverviewShowsTheNoteAtTheTop(test *testing.T) {
	composer := noteComposer(test)
	if strings.Contains(composer.render(), "Private note") {
		test.Error("a pull request without a note shows the section")
	}
	composer.withNote("remember to check the **jitter** bounds")
	rendered := composer.render()
	note := strings.Index(rendered, "Private note (only on this computer)")
	description := strings.Index(rendered, "Description")
	if note < 0 || !strings.Contains(rendered, "jitter") || description < note {
		test.Errorf("the note is not above the description:\n%s", rendered)
	}
}

func TestOverviewIgnoresTheNoteOfAnotherPullRequest(test *testing.T) {
	composer := noteComposer(test)
	other := noteKey
	other.Number = 8
	notes, err := pullrequestnotes.Notes{}.Set(other, "not this one")
	if err != nil {
		test.Fatal(err)
	}
	composer.send(userinterface.NotesMessage{Notes: notes})
	if strings.Contains(composer.render(), "Private note") {
		test.Error("another pull request's note is shown")
	}
}
