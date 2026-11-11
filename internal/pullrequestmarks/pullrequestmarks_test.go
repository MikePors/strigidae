package pullrequestmarks

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func key(host, owner, name string, number int) Key {
	return Key{Host: host, Repository: github.Repository{Owner: owner, Name: name}, Number: number}
}

var first = key("github.com", "octo", "hello", 1)

func marksPath(directory string) string {
	return filepath.Join(directory, "strig", "pull-request-marks.json")
}

func mustOpen(test *testing.T, directory string) *Store {
	test.Helper()
	store, err := Open(directory)
	if err != nil {
		test.Fatalf("Open: %v", err)
	}
	return store
}

func TestSetReplaceClear(test *testing.T) {
	store := mustOpen(test, "")
	if _, ok := store.Mark(first); ok {
		test.Fatal("a new store has a mark")
	}
	if err := store.Set(first, "  waiting on author "); err != nil {
		test.Fatalf("Set: %v", err)
	}
	if text, ok := store.Mark(first); !ok || text != "waiting on author" {
		test.Errorf("Mark = %q, %v; want the trimmed text", text, ok)
	}
	if err := store.Set(first, "left for Sam"); err != nil {
		test.Fatalf("Set: %v", err)
	}
	if diff := cmp.Diff([]string{"left for Sam"}, store.Names()); diff != "" {
		test.Errorf("Names after replacing (-want +got):\n%s", diff)
	}
	store.Clear(first)
	if _, ok := store.Mark(first); ok {
		test.Error("Clear left the mark")
	}
	store.Clear(first) // clearing an unmarked pull request is fine
	if len(store.Names()) != 0 {
		test.Errorf("Names after clearing = %v, want none", store.Names())
	}
}

func TestKeysDoNotCollide(test *testing.T) {
	keys := []Key{
		key("github.com", "octo", "hello", 1),
		key("github.com", "octo", "hello", 2),
		key("github.com", "octo", "other", 1),
		key("github.com", "other", "hello", 1),
		key("git.example.com", "octo", "hello", 1),
	}
	store := mustOpen(test, "")
	for index, current := range keys {
		if err := store.Set(current, "mark "+strconv.Itoa(index)); err != nil {
			test.Fatalf("Set %v: %v", current, err)
		}
	}
	for index, current := range keys {
		if text, _ := store.Mark(current); text != "mark "+strconv.Itoa(index) {
			test.Errorf("Mark(%v) = %q, want %q", current, text, "mark "+strconv.Itoa(index))
		}
	}
	store.Clear(keys[0])
	if _, ok := store.Mark(keys[1]); !ok {
		test.Error("clearing one pull request cleared another")
	}
}

func TestKeyIgnoresCase(test *testing.T) {
	store := mustOpen(test, "")
	if err := store.Set(key("GitHub.com", "Octo", "Hello", 1), "mine"); err != nil {
		test.Fatalf("Set: %v", err)
	}
	if text, ok := store.Mark(first); !ok || text != "mine" {
		test.Errorf("Mark = %q, %v; want the mark set under another spelling", text, ok)
	}
}

func TestNamesDistinctAndSorted(test *testing.T) {
	store := mustOpen(test, "")
	for number, text := range map[int]string{1: "zebra", 2: "apple", 3: "zebra", 4: "mango"} {
		if err := store.Set(key("github.com", "octo", "hello", number), text); err != nil {
			test.Fatalf("Set: %v", err)
		}
	}
	if diff := cmp.Diff([]string{"apple", "mango", "zebra"}, store.Names()); diff != "" {
		test.Errorf("Names (-want +got):\n%s", diff)
	}
}

func TestSetRefusals(test *testing.T) {
	tests := []struct {
		name string
		key  Key
		text string
		want error
	}{
		{"empty", first, "", ErrEmpty},
		{"blank", first, " \t ", ErrEmpty},
		{"newline", first, "two\nlines", ErrControl},
		{"escape", first, "red\x1b[31m", ErrControl},
		{"delete", first, "a\x7fb", ErrControl},
		{"c1 control", first, "a\u009bb", ErrControl},
		{"bidi override", first, "abc\u202edef", ErrControl},
		{"bidi isolate", first, "abc\u2066def", ErrControl},
		{"too long", first, strings.Repeat("a", MaxLength+1), ErrTooLong},
		{"too long in characters", first, strings.Repeat("é", MaxLength+1), ErrTooLong},
		{"no host", key("", "octo", "hello", 1), "ok", ErrInvalidKey},
		{"no owner", key("github.com", "", "hello", 1), "ok", ErrInvalidKey},
		{"no name", key("github.com", "octo", "", 1), "ok", ErrInvalidKey},
		{"no number", key("github.com", "octo", "hello", 0), "ok", ErrInvalidKey},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			store := mustOpen(test, "")
			if err := store.Set(testCase.key, testCase.text); !errors.Is(err, testCase.want) {
				test.Errorf("Set error = %v, want %v", err, testCase.want)
			}
			if len(store.Names()) != 0 {
				test.Errorf("a refused mark was kept: %v", store.Names())
			}
		})
	}
}

func TestSetAcceptsLimits(test *testing.T) {
	store := mustOpen(test, "")
	for _, text := range []string{strings.Repeat("a", MaxLength), strings.Repeat("é", MaxLength), "waiting on 🦉"} {
		if err := store.Set(first, text); err != nil {
			test.Errorf("Set(%q): %v", text, err)
		}
	}
}

func TestRoundTrip(test *testing.T) {
	directory := test.TempDir()
	store := mustOpen(test, directory)
	marks := map[Key]string{
		first: "waiting on author",
		key("git.example.com", "octo", "hello", 1): "left for Sam",
		key("github.com", "octo", "<hello>", 7):    "a & b <c>",
	}
	for current, text := range marks {
		if err := store.Set(current, text); err != nil {
			test.Fatalf("Set: %v", err)
		}
	}
	if err := store.Save(); err != nil {
		test.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(marksPath(directory))
	if err != nil {
		test.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		test.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}

	reopened := mustOpen(test, directory)
	for current, text := range marks {
		if got, ok := reopened.Mark(current); !ok || got != text {
			test.Errorf("Mark(%v) = %q, %v; want %q", current, got, ok, text)
		}
	}

	reopened.Clear(first)
	if err := reopened.Save(); err != nil {
		test.Fatalf("Save: %v", err)
	}
	if _, ok := mustOpen(test, directory).Mark(first); ok {
		test.Error("a cleared mark came back")
	}
	entries, err := os.ReadDir(filepath.Dir(marksPath(directory)))
	if err != nil {
		test.Fatal(err)
	}
	if len(entries) != 1 {
		test.Errorf("directory holds %d entries, want only the marks file", len(entries))
	}
}

func TestSaveIsStable(test *testing.T) {
	directory := test.TempDir()
	store := mustOpen(test, directory)
	for number := 1; number <= 20; number++ {
		if err := store.Set(key("github.com", "octo", "hello", number), "mark"); err != nil {
			test.Fatal(err)
		}
	}
	var saved [][]byte
	for range 2 {
		if err := store.Save(); err != nil {
			test.Fatal(err)
		}
		data, err := os.ReadFile(marksPath(directory))
		if err != nil {
			test.Fatal(err)
		}
		saved = append(saved, data)
	}
	if string(saved[0]) != string(saved[1]) {
		test.Error("two saves of the same marks differ")
	}
}

func TestMissingFile(test *testing.T) {
	directory := test.TempDir()
	store := mustOpen(test, directory)
	if len(store.Names()) != 0 {
		test.Errorf("Names = %v, want none", store.Names())
	}
	if _, err := os.Stat(marksPath(directory)); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("Open created the file: %v", err)
	}
}

func TestUnusableFileIsNeverOverwritten(test *testing.T) {
	valid := `{"version":1,"marks":[{"host":"github.com","owner":"octo","repository":"hello","number":1,"text":"x"}]}`
	tests := []struct {
		name    string
		content string
		want    error
	}{
		{"not json", "this is not json", ErrCorrupt},
		{"empty file", "", ErrCorrupt},
		{"truncated", valid[:len(valid)-5], ErrCorrupt},
		{"wrong types", `{"version":1,"marks":"none"}`, ErrCorrupt},
		{"unknown version", `{"version":2,"marks":[]}`, ErrUnknownVersion},
		{"no version", `{"marks":[]}`, ErrUnknownVersion},
		{"mark without a number", `{"version":1,"marks":[{"host":"github.com","owner":"octo","repository":"hello","text":"x"}]}`, ErrCorrupt},
		{"untrimmed text", `{"version":1,"marks":[{"host":"github.com","owner":"octo","repository":"hello","number":1,"text":" x "}]}`, ErrCorrupt},
		{"duplicate keys differing in case", `{"version":1,"marks":[{"host":"github.com","owner":"Octo","repository":"hello","number":1,"text":"x"},{"host":"github.com","owner":"octo","repository":"hello","number":1,"text":"y"}]}`, ErrCorrupt},
		{"refused text", `{"version":1,"marks":[{"host":"github.com","owner":"octo","repository":"hello","number":1,"text":"a\u001bb"}]}`, ErrCorrupt},
		{"too large", `{"version":1,"marks":[],"padding":"` + strings.Repeat("a", MaxFileSize) + `"}`, ErrTooLarge},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			directory := test.TempDir()
			if err := os.MkdirAll(filepath.Dir(marksPath(directory)), 0o700); err != nil {
				test.Fatal(err)
			}
			if err := os.WriteFile(marksPath(directory), []byte(testCase.content), 0o600); err != nil {
				test.Fatal(err)
			}
			store, err := Open(directory)
			if !errors.Is(err, testCase.want) {
				test.Fatalf("Open error = %v, want %v", err, testCase.want)
			}
			if store == nil {
				test.Fatal("Open returned no store with its error")
			}
			if err := store.Set(first, "new"); err != nil {
				test.Fatalf("Set on an unusable store: %v", err)
			}
			if err := store.Save(); !errors.Is(err, testCase.want) {
				test.Errorf("Save error = %v, want %v", err, testCase.want)
			}
			data, err := os.ReadFile(marksPath(directory))
			if err != nil {
				test.Fatal(err)
			}
			if string(data) != testCase.content {
				test.Error("the unusable file was overwritten")
			}
		})
	}
}

func TestOpenNormalisesKeys(test *testing.T) {
	directory := test.TempDir()
	if err := os.MkdirAll(filepath.Dir(marksPath(directory)), 0o700); err != nil {
		test.Fatal(err)
	}
	content := `{"version":1,"marks":[{"host":"GitHub.com","owner":"Octo","repository":"Hello","number":1,"text":"waiting"}]}`
	if err := os.WriteFile(marksPath(directory), []byte(content), 0o600); err != nil {
		test.Fatal(err)
	}
	store := mustOpen(test, directory)
	if text, ok := store.Mark(key("github.com", "octo", "hello", 1)); !ok || text != "waiting" {
		test.Errorf("Mark = %q, %v, want waiting, true", text, ok)
	}
}

func TestOpenReturnsPermissionError(test *testing.T) {
	if os.Geteuid() == 0 {
		test.Skip("root ignores file permissions")
	}
	directory := test.TempDir()
	if err := os.MkdirAll(filepath.Dir(marksPath(directory)), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(marksPath(directory), []byte(`{"version":1,"marks":[]}`), 0o000); err != nil {
		test.Fatal(err)
	}
	if _, err := Open(directory); !errors.Is(err, os.ErrPermission) {
		test.Errorf("Open error = %v, want a permission error", err)
	}
}

func TestSavePermissionErrorIsReturned(test *testing.T) {
	if os.Geteuid() == 0 {
		test.Skip("root ignores file permissions")
	}
	directory := test.TempDir()
	store := mustOpen(test, directory)
	if err := os.Mkdir(filepath.Join(directory, "strig"), 0o500); err != nil {
		test.Fatal(err)
	}
	if err := store.Set(first, "x"); err != nil {
		test.Fatal(err)
	}
	if err := store.Save(); !errors.Is(err, os.ErrPermission) {
		test.Errorf("Save error = %v, want a permission error", err)
	}
}

func TestNoStateDirectoryKeepsMarksForTheRun(test *testing.T) {
	store := mustOpen(test, "")
	if err := store.Set(first, "x"); err != nil {
		test.Fatal(err)
	}
	if err := store.Save(); err != nil {
		test.Errorf("Save without a directory = %v, want nil", err)
	}
	if text, _ := store.Mark(first); text != "x" {
		test.Errorf("Mark = %q, want x", text)
	}
}

// TestConcurrentUse is for the race detector: Update changes the store while a
// command saves it. It covers data races on the marks only; that an older
// snapshot is never written after a newer one rests on Save taking its
// snapshot under the save mutex, which this test can't observe.
func TestConcurrentUse(test *testing.T) {
	store := mustOpen(test, test.TempDir())
	var group sync.WaitGroup
	for worker := range 4 {
		group.Add(2)
		go func() {
			defer group.Done()
			for number := 1; number <= 25; number++ {
				current := key("github.com", "octo", "hello", worker*100+number)
				_ = store.Set(current, "mark")
				_, _ = store.Mark(current)
				_ = store.Names()
				store.Clear(current)
				_ = store.Set(current, "again")
			}
		}()
		go func() {
			defer group.Done()
			for range 10 {
				if err := store.Save(); err != nil {
					test.Errorf("Save: %v", err)
				}
			}
		}()
	}
	group.Wait()
	if err := store.Save(); err != nil {
		test.Fatal(err)
	}
	if got := len(store.marks); got != 100 {
		test.Errorf("%d marks, want 100", got)
	}
}
