package pullrequestnotes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func key(host, owner, name string, number int) Key {
	return Key{Host: host, Repository: github.Repository{Owner: owner, Name: name}, Number: number}
}

var first = key("github.com", "octo", "hello", 1)

func mustSet(test *testing.T, notes Notes, current Key, text string) Notes {
	test.Helper()
	changed, err := notes.Set(current, text)
	if err != nil {
		test.Fatalf("Set(%v, %q): %v", current, text, err)
	}
	return changed
}

func TestZeroValueIsUsable(test *testing.T) {
	var notes Notes
	if _, ok := notes.Note(first); ok {
		test.Error("the zero value has a note")
	}
	if !notes.Empty() || !notes.Clear(first).Empty() {
		test.Error("the zero value is not empty")
	}
	if text, ok := mustSet(test, notes, first, "x").Note(first); !ok || text != "x" {
		test.Errorf("Note after Set on the zero value = %q, %v", text, ok)
	}
}

func TestSetKeepsLinesAndTrimsTheEnds(test *testing.T) {
	notes := mustSet(test, Notes{}, first, "\n  first line\r\n\tsecond\n\nthird  \n\n")
	text, _ := notes.Note(first)
	if diff := cmp.Diff("first line\n\tsecond\n\nthird", text); diff != "" {
		test.Errorf("Note (-want +got):\n%s", diff)
	}
}

func TestReplaceAndClear(test *testing.T) {
	notes := mustSet(test, Notes{}, first, "one")
	notes = mustSet(test, notes, first, "two")
	if text, _ := notes.Note(first); text != "two" {
		test.Errorf("Note after replacing = %q", text)
	}
	if _, ok := notes.Clear(first).Note(first); ok {
		test.Error("Clear left the note")
	}
}

func TestChangesLeaveTheReceiverAlone(test *testing.T) {
	original := mustSet(test, Notes{}, first, "one")
	_ = mustSet(test, original, first, "two")
	_ = original.Clear(first)
	if text, ok := original.Note(first); !ok || text != "one" {
		test.Errorf("receiver changed: %q, %v", text, ok)
	}
}

func TestKeysAreSeparateAndIgnoreCase(test *testing.T) {
	notes := mustSet(test, Notes{}, first, "a")
	notes = mustSet(test, notes, key("github.com", "octo", "hello", 2), "b")
	notes = mustSet(test, notes, key("example.com", "octo", "hello", 1), "c")
	for _, pair := range []struct {
		key  Key
		want string
	}{
		{first, "a"},
		{key("github.com", "OCTO", "Hello", 1), "a"},
		{key("github.com", "octo", "hello", 2), "b"},
		{key("example.com", "octo", "hello", 1), "c"},
	} {
		if text, _ := notes.Note(pair.key); text != pair.want {
			test.Errorf("Note(%v) = %q, want %q", pair.key, text, pair.want)
		}
	}
}

func TestSetRefusals(test *testing.T) {
	for _, testCase := range []struct {
		name string
		key  Key
		text string
		want error
	}{
		{"empty", first, "", ErrEmpty},
		{"blank", first, " \n\t ", ErrEmpty},
		{"no host", key("", "octo", "hello", 1), "x", ErrInvalidKey},
		{"no number", key("github.com", "octo", "hello", 0), "x", ErrInvalidKey},
		{"escape", first, "a\x1b[31mb", ErrControl},
		{"null", first, "a\x00b", ErrControl},
		{"bidirectional", first, "a\u202eb", ErrControl},
		{"invalid UTF-8", first, "a\xffb", ErrInvalidUTF8},
		{"too large", first, strings.Repeat("a", MaxNoteSize+1), ErrTooLarge},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			notes, err := Notes{}.Set(testCase.key, testCase.text)
			if !errors.Is(err, testCase.want) {
				test.Errorf("error = %v, want %v", err, testCase.want)
			}
			if !notes.Empty() {
				test.Error("a refused Set changed the notes")
			}
		})
	}
	if _, err := (Notes{}).Set(first, strings.Repeat("a", MaxNoteSize)); err != nil {
		test.Errorf("a note of exactly MaxNoteSize: %v", err)
	}
}

func TestEncodeDecodeRoundTrip(test *testing.T) {
	notes := mustSet(test, Notes{}, first, "line one\nline two <b> & \"quoted\"\n\n- list")
	notes = mustSet(test, notes, key("github.com", "octo", "hello", 2), "ünïcode ✓")
	data, err := Encode(notes)
	if err != nil {
		test.Fatal(err)
	}
	if strings.Contains(string(data), "\\u003c") {
		test.Error("HTML characters were escaped")
	}
	decoded, err := Decode(data)
	if err != nil {
		test.Fatal(err)
	}
	for _, current := range []Key{first, key("github.com", "octo", "hello", 2)} {
		want, _ := notes.Note(current)
		if got, _ := decoded.Note(current); got != want {
			test.Errorf("Note(%v) = %q, want %q", current, got, want)
		}
	}
}

func TestEncodeIsStable(test *testing.T) {
	second := key("github.com", "octo", "hello", 2)
	one := mustSet(test, mustSet(test, Notes{}, first, "a"), second, "b")
	other := mustSet(test, mustSet(test, Notes{}, second, "b"), first, "a")
	oneData, _ := Encode(one)
	otherData, _ := Encode(other)
	if string(oneData) != string(otherData) {
		test.Errorf("equal notes encode differently:\n%s\n%s", oneData, otherData)
	}
}

func TestDecodeRefusals(test *testing.T) {
	for _, testCase := range []struct {
		name string
		data string
		want error
	}{
		{"not JSON", `nope`, ErrCorrupt},
		{"unknown version", `{"version":2,"notes":[]}`, ErrUnknownVersion},
		{"no version", `{"notes":[]}`, ErrUnknownVersion},
		{"bad shape", `{"version":1,"notes":"x"}`, ErrCorrupt},
		{"empty text", `{"version":1,"notes":[{"host":"h","owner":"o","repository":"r","number":1,"text":""}]}`, ErrCorrupt},
		{"untrimmed text", `{"version":1,"notes":[{"host":"h","owner":"o","repository":"r","number":1,"text":" x"}]}`, ErrCorrupt},
		{"control character", `{"version":1,"notes":[{"host":"h","owner":"o","repository":"r","number":1,"text":"a\u001bb"}]}`, ErrCorrupt},
		{"bad key", `{"version":1,"notes":[{"host":"","owner":"o","repository":"r","number":1,"text":"x"}]}`, ErrCorrupt},
		{"duplicate by case", `{"version":1,"notes":[{"host":"h","owner":"o","repository":"r","number":1,"text":"x"},{"host":"h","owner":"O","repository":"r","number":1,"text":"y"}]}`, ErrCorrupt},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			notes, err := Decode([]byte(testCase.data))
			if !errors.Is(err, testCase.want) {
				test.Errorf("error = %v, want %v", err, testCase.want)
			}
			if !notes.Empty() {
				test.Error("a refused file gave notes")
			}
		})
	}
}

func TestPath(test *testing.T) {
	if got, want := Path("/state"), filepath.Join("/state", "strig", "pull-request-notes.json"); got != want {
		test.Errorf("Path = %q, want %q", got, want)
	}
}

func TestReadMissingFileHoldsNoNotes(test *testing.T) {
	notes, err := Read(filepath.Join(test.TempDir(), "missing.json"))
	if err != nil || !notes.Empty() {
		test.Errorf("Read of a missing file = %v, %v", notes, err)
	}
}

func TestWriteThenRead(test *testing.T) {
	path := Path(test.TempDir())
	notes := mustSet(test, Notes{}, first, "kept\nbetween runs")
	data, err := Encode(notes)
	if err != nil {
		test.Fatal(err)
	}
	if err := Write(path, data); err != nil {
		test.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		test.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		test.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		test.Errorf("directory holds %d files, want only the notes file", len(entries))
	}
	read, err := Read(path)
	if err != nil {
		test.Fatal(err)
	}
	if text, _ := read.Note(first); text != "kept\nbetween runs" {
		test.Errorf("Note after reading = %q", text)
	}
}

func TestReadRefusesBadFilesAndNamesThePath(test *testing.T) {
	path := filepath.Join(test.TempDir(), "notes.json")
	if err := os.WriteFile(path, []byte("garbage"), fileMode); err != nil {
		test.Fatal(err)
	}
	_, err := Read(path)
	if !errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), path) {
		test.Errorf("error = %v, want ErrCorrupt naming the path", err)
	}
	if strings.Contains(err.Error(), "garbage") {
		test.Error("the error holds the file's content")
	}
}

func TestCleanTurnsEveryLineEndingIntoLF(test *testing.T) {
	if diff := cmp.Diff("a\nb\nc\nd", Clean("a\r\nb\rc\nd\r")); diff != "" {
		test.Errorf("Clean (-want +got):\n%s", diff)
	}
	if err := Validate("pasted\rwith an old line ending"); err != nil {
		test.Errorf("Validate refused a lone carriage return: %v", err)
	}
}

func TestValidateAgreesWithSet(test *testing.T) {
	for _, text := range []string{"", " \n", "ok", "a\x1bb", "a\u200fb", "a\x0cb", "a\xffb", strings.Repeat("a", MaxNoteSize+1)} {
		_, setErr := Notes{}.Set(first, text)
		if validateErr := Validate(text); (validateErr == nil) != (setErr == nil) || (setErr != nil && !errors.Is(validateErr, setErr)) {
			test.Errorf("Validate(%.10q) = %v, Set = %v", text, validateErr, setErr)
		}
	}
}

func TestEncodeRefusesAFileReadWouldRefuse(test *testing.T) {
	text := strings.Repeat("a", MaxNoteSize)
	notes := Notes{}
	for number := 1; number <= MaxFileSize/MaxNoteSize+1; number++ {
		notes = mustSet(test, notes, key("github.com", "octo", "hello", number), text)
	}
	if _, err := Encode(notes); !errors.Is(err, ErrFileTooLarge) {
		test.Errorf("Encode of %d notes of the largest size = %v, want ErrFileTooLarge", MaxFileSize/MaxNoteSize+1, err)
	}
}

func TestSaverSkipsAnOlderGenerationAndRetriesAFailedWrite(test *testing.T) {
	path := Path(test.TempDir())
	saver := NewSaver(path)
	if err := saver.Save(2, []byte(`{"version":1,"notes":[]}`+"\n")); err != nil {
		test.Fatal(err)
	}
	newer, _ := Encode(mustSet(test, Notes{}, first, "newer"))
	if err := saver.Save(3, newer); err != nil {
		test.Fatal(err)
	}
	older, _ := Encode(mustSet(test, Notes{}, first, "older"))
	if err := saver.Save(1, older); err != nil || saver.Save(3, older) != nil {
		test.Fatal("a skipped save reported an error")
	}
	read, err := Read(path)
	if err != nil {
		test.Fatal(err)
	}
	if text, _ := read.Note(first); text != "newer" {
		test.Errorf("Note = %q, want the newest generation's", text)
	}

	// A write that fails does not count as written, so the same generation can be tried again.
	blocked := NewSaver(filepath.Join(test.TempDir(), "file", "notes.json"))
	if err := os.WriteFile(filepath.Dir(blocked.path), nil, 0o600); err != nil {
		test.Fatal(err)
	}
	if err := blocked.Save(1, newer); err == nil {
		test.Fatal("a write into a file that is no directory succeeded")
	}
	if err := os.Remove(filepath.Dir(blocked.path)); err != nil {
		test.Fatal(err)
	}
	if err := blocked.Save(1, newer); err != nil {
		test.Errorf("retry after the failure: %v", err)
	}
}
