package pullrequestmarks

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

func key(host, owner, name string, number int) Key {
	return Key{Host: host, Repository: github.Repository{Owner: owner, Name: name}, Number: number}
}

var first = key("github.com", "octo", "hello", 1)

func mustSet(test *testing.T, marks Marks, current Key, text string) Marks {
	test.Helper()
	changed, err := marks.Set(current, text)
	if err != nil {
		test.Fatalf("Set(%v, %q): %v", current, text, err)
	}
	return changed
}

func TestZeroValueIsUsable(test *testing.T) {
	var marks Marks
	if _, ok := marks.Mark(first); ok {
		test.Error("the zero value has a mark")
	}
	if len(marks.Names()) != 0 {
		test.Errorf("Names = %v, want none", marks.Names())
	}
	if _, ok := marks.Clear(first).Mark(first); ok {
		test.Error("Clear on the zero value made a mark")
	}
	changed := mustSet(test, marks, first, "x")
	if text, ok := changed.Mark(first); !ok || text != "x" {
		test.Errorf("Mark after Set on the zero value = %q, %v", text, ok)
	}
}

func TestSetReplaceClear(test *testing.T) {
	marks := mustSet(test, Marks{}, first, "  waiting on author ")
	if text, ok := marks.Mark(first); !ok || text != "waiting on author" {
		test.Errorf("Mark = %q, %v; want the trimmed text", text, ok)
	}
	marks = mustSet(test, marks, first, "left for Sam")
	if diff := cmp.Diff([]string{"left for Sam"}, marks.Names()); diff != "" {
		test.Errorf("Names after replacing (-want +got):\n%s", diff)
	}
	marks = marks.Clear(first)
	if _, ok := marks.Mark(first); ok {
		test.Error("Clear left the mark")
	}
	marks = marks.Clear(first) // clearing an unmarked pull request is fine
	if len(marks.Names()) != 0 {
		test.Errorf("Names after clearing = %v, want none", marks.Names())
	}
}

func TestChangesLeaveTheReceiverAlone(test *testing.T) {
	second := key("github.com", "octo", "hello", 2)
	original := mustSet(test, mustSet(test, Marks{}, first, "one"), second, "two")
	before, err := Encode(original)
	if err != nil {
		test.Fatal(err)
	}

	tests := []struct {
		name   string
		change func(Marks) Marks
		want   map[Key]string // the changed value
	}{
		{"replace", func(marks Marks) Marks { return mustSet(test, marks, first, "changed") }, map[Key]string{first: "changed", second: "two"}},
		{"add", func(marks Marks) Marks { return mustSet(test, marks, key("github.com", "octo", "hello", 3), "three") }, map[Key]string{first: "one", second: "two", key("github.com", "octo", "hello", 3): "three"}},
		{"clear", func(marks Marks) Marks { return marks.Clear(first) }, map[Key]string{second: "two"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			changed := testCase.change(original)
			for current, want := range testCase.want {
				if got, ok := changed.Mark(current); !ok || got != want {
					test.Errorf("changed Mark(%v) = %q, %v; want %q", current, got, ok, want)
				}
			}
			if len(changed.Names()) > len(testCase.want) {
				test.Errorf("changed has more marks than %d: %v", len(testCase.want), changed.Names())
			}
			after, err := Encode(original)
			if err != nil {
				test.Fatal(err)
			}
			if string(after) != string(before) {
				test.Errorf("the receiver changed:\n%s\nwant\n%s", after, before)
			}
		})
	}
}

func TestRefusedSetLeavesMarksAlone(test *testing.T) {
	marks := mustSet(test, Marks{}, first, "kept")
	got, err := marks.Set(first, "")
	if !errors.Is(err, ErrEmpty) {
		test.Fatalf("Set error = %v, want %v", err, ErrEmpty)
	}
	if text, ok := got.Mark(first); !ok || text != "kept" {
		test.Errorf("Mark after a refused Set = %q, %v; want the old mark", text, ok)
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
	var marks Marks
	for index, current := range keys {
		marks = mustSet(test, marks, current, "mark "+strconv.Itoa(index))
	}
	for index, current := range keys {
		if text, _ := marks.Mark(current); text != "mark "+strconv.Itoa(index) {
			test.Errorf("Mark(%v) = %q, want %q", current, text, "mark "+strconv.Itoa(index))
		}
	}
	marks = marks.Clear(keys[0])
	if _, ok := marks.Mark(keys[1]); !ok {
		test.Error("clearing one pull request cleared another")
	}
}

func TestKeyIgnoresCase(test *testing.T) {
	marks := mustSet(test, Marks{}, key("GitHub.com", "Octo", "Hello", 1), "mine")
	if text, ok := marks.Mark(first); !ok || text != "mine" {
		test.Errorf("Mark = %q, %v; want the mark set under another spelling", text, ok)
	}
	if _, ok := marks.Clear(key("GITHUB.COM", "OCTO", "HELLO", 1)).Mark(first); ok {
		test.Error("Clear under another spelling left the mark")
	}
}

func TestNamesDistinctAndSorted(test *testing.T) {
	var marks Marks
	for number, text := range map[int]string{1: "zebra", 2: "apple", 3: "zebra", 4: "mango"} {
		marks = mustSet(test, marks, key("github.com", "octo", "hello", number), text)
	}
	if diff := cmp.Diff([]string{"apple", "mango", "zebra"}, marks.Names()); diff != "" {
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
		{"invalid UTF-8", first, "a\xffb", ErrInvalidUTF8},
		{"too long", first, strings.Repeat("a", MaxLength+1), ErrTooLong},
		{"too long in characters", first, strings.Repeat("é", MaxLength+1), ErrTooLong},
		{"no host", key("", "octo", "hello", 1), "ok", ErrInvalidKey},
		{"no owner", key("github.com", "", "hello", 1), "ok", ErrInvalidKey},
		{"no name", key("github.com", "octo", "", 1), "ok", ErrInvalidKey},
		{"no number", key("github.com", "octo", "hello", 0), "ok", ErrInvalidKey},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			marks, err := Marks{}.Set(testCase.key, testCase.text)
			if !errors.Is(err, testCase.want) {
				test.Errorf("Set error = %v, want %v", err, testCase.want)
			}
			if len(marks.Names()) != 0 {
				test.Errorf("a refused mark was kept: %v", marks.Names())
			}
		})
	}
}

func TestSetAcceptsLimits(test *testing.T) {
	for _, text := range []string{strings.Repeat("a", MaxLength), strings.Repeat("é", MaxLength), "waiting on 🦉"} {
		if _, err := (Marks{}).Set(first, text); err != nil {
			test.Errorf("Set(%q): %v", text, err)
		}
	}
}

func TestEncodeDecodeRoundTrip(test *testing.T) {
	want := map[Key]string{
		first: "waiting on author",
		key("git.example.com", "octo", "hello", 1): "left for Sam",
		key("github.com", "octo", "<hello>", 7):    "a & b <c>",
	}
	var marks Marks
	for current, text := range want {
		marks = mustSet(test, marks, current, text)
	}
	data, err := Encode(marks)
	if err != nil {
		test.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(data), "\\u0026") || strings.Contains(string(data), "\\u003c") {
		test.Errorf("Encode escaped HTML characters: %s", data)
	}
	decoded, err := Decode(data)
	if err != nil {
		test.Fatalf("Decode: %v", err)
	}
	for current, text := range want {
		if got, ok := decoded.Mark(current); !ok || got != text {
			test.Errorf("Mark(%v) = %q, %v; want %q", current, got, ok, text)
		}
	}
	if len(decoded.Names()) != 3 {
		test.Errorf("Names = %v, want 3 marks", decoded.Names())
	}
}

func TestEncodeIsStable(test *testing.T) {
	var marks Marks
	for number := 1; number <= 20; number++ {
		marks = mustSet(test, marks, key("github.com", "octo", "hello", number), "mark")
	}
	var encoded []string
	for range 5 {
		data, err := Encode(marks)
		if err != nil {
			test.Fatal(err)
		}
		encoded = append(encoded, string(data))
	}
	for _, other := range encoded[1:] {
		if other != encoded[0] {
			test.Fatal("two encodings of the same marks differ")
		}
	}
	// The entries come out sorted by number, not in map order.
	last := -1
	for number := 1; number <= 20; number++ {
		index := strings.Index(encoded[0], `"number":`+strconv.Itoa(number)+`,`)
		if index <= last {
			test.Fatalf("entry %d is out of order", number)
		}
		last = index
	}
}

func TestEncodeEmpty(test *testing.T) {
	data, err := Encode(Marks{})
	if err != nil {
		test.Fatal(err)
	}
	if got, want := string(data), `{"version":1,"marks":[]}`+"\n"; got != want {
		test.Errorf("Encode(Marks{}) = %q, want %q", got, want)
	}
	if _, err := Decode(data); err != nil {
		test.Errorf("Decode of empty marks: %v", err)
	}
}

func TestDecodeNormalisesKeys(test *testing.T) {
	content := `{"version":1,"marks":[{"host":"GitHub.com","owner":"Octo","repository":"Hello","number":1,"text":"waiting"}]}`
	marks, err := Decode([]byte(content))
	if err != nil {
		test.Fatal(err)
	}
	if text, ok := marks.Mark(first); !ok || text != "waiting" {
		test.Errorf("Mark = %q, %v, want waiting, true", text, ok)
	}
}

func TestDecodeRefusals(test *testing.T) {
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
		{"a good entry before a bad one", `{"version":1,"marks":[{"host":"github.com","owner":"octo","repository":"hello","number":1,"text":"x"},{"host":"github.com","owner":"octo","repository":"hello","number":2,"text":""}]}`, ErrCorrupt},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			marks, err := Decode([]byte(testCase.content))
			if !errors.Is(err, testCase.want) {
				test.Fatalf("Decode error = %v, want %v", err, testCase.want)
			}
			if names := marks.Names(); len(names) != 0 {
				test.Errorf("Decode kept marks %v with its error", names)
			}
			if _, ok := marks.Mark(key("github.com", "octo", "hello", 1)); ok {
				test.Error("Decode kept the first entry with its error")
			}
		})
	}
}

func TestPath(test *testing.T) {
	if got, want := Path("/state"), filepath.Join("/state", "strig", "pull-request-marks.json"); got != want {
		test.Errorf("Path = %q, want %q", got, want)
	}
}

func TestReadMissingFile(test *testing.T) {
	path := Path(test.TempDir())
	marks, err := Read(path)
	if err != nil {
		test.Fatalf("Read of a missing file: %v", err)
	}
	if len(marks.Names()) != 0 {
		test.Errorf("Names = %v, want none", marks.Names())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		test.Errorf("Read created the file: %v", err)
	}
}

func TestWriteThenRead(test *testing.T) {
	path := Path(test.TempDir())
	marks := mustSet(test, mustSet(test, Marks{}, first, "waiting"), key("git.example.com", "octo", "hello", 2), "left for Sam")
	data, err := Encode(marks)
	if err != nil {
		test.Fatal(err)
	}
	if err := Write(path, data); err != nil {
		test.Fatalf("Write: %v", err)
	}
	read, err := Read(path)
	if err != nil {
		test.Fatalf("Read: %v", err)
	}
	if diff := cmp.Diff(marks.Names(), read.Names()); diff != "" {
		test.Errorf("Names (-want +got):\n%s", diff)
	}
	if text, ok := read.Mark(first); !ok || text != "waiting" {
		test.Errorf("Mark = %q, %v", text, ok)
	}
}

func TestWriteModeAndNoLeftovers(test *testing.T) {
	directory := test.TempDir()
	path := Path(directory)
	for _, content := range []string{"first", "second"} { // the second replaces the first
		if err := Write(path, []byte(content)); err != nil {
			test.Fatalf("Write: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "second" {
		test.Fatalf("file = %q, %v; want the second write", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		test.Fatal(err)
	}
	if info.Mode().Perm() != fileMode {
		test.Errorf("file mode = %v, want %v", info.Mode().Perm(), os.FileMode(fileMode))
	}
	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		test.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != directoryMode {
		test.Errorf("directory mode = %v, want %v", directoryInfo.Mode().Perm(), os.FileMode(directoryMode))
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		test.Fatal(err)
	}
	if len(entries) != 1 {
		test.Errorf("directory holds %d entries, want only the marks file", len(entries))
	}
}

func TestWriteFailureLeavesFileAndNoTemporaryFile(test *testing.T) {
	directory := test.TempDir()
	path := filepath.Join(directory, "marks.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		test.Fatal(err)
	}
	// Renaming a file over a directory fails after the temporary file is written.
	blocked := filepath.Join(directory, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "inner"), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := Write(blocked, []byte("new")); err == nil {
		test.Fatal("Write over a directory succeeded")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		test.Fatal(err)
	}
	if len(entries) != 2 {
		test.Errorf("directory holds %d entries, want the old file and the directory only", len(entries))
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		test.Errorf("unrelated file = %q", data)
	}
}

// paddedFile returns a valid, empty marks file of exactly size bytes.
func paddedFile(size int) []byte {
	prefix, suffix := `{"version":1,"marks":[],"padding":"`, `"}`
	return []byte(prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix)
}

func TestReadSizeLimit(test *testing.T) {
	path := Path(test.TempDir())
	if err := Write(path, paddedFile(MaxFileSize)); err != nil {
		test.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		test.Errorf("Read of a file of exactly MaxFileSize bytes: %v", err)
	}
	if err := Write(path, paddedFile(MaxFileSize+1)); err != nil {
		test.Fatal(err)
	}
	marks, err := Read(path)
	if !errors.Is(err, ErrTooLarge) {
		test.Fatalf("Read error = %v, want %v", err, ErrTooLarge)
	}
	if !strings.Contains(err.Error(), path) {
		test.Errorf("Read error %q does not name the file", err)
	}
	if len(marks.Names()) != 0 {
		test.Errorf("Read kept marks %v with its error", marks.Names())
	}
}

func TestReadDecodesTheFile(test *testing.T) {
	path := Path(test.TempDir())
	if err := Write(path, []byte("this is not json")); err != nil {
		test.Fatal(err)
	}
	_, err := Read(path)
	if !errors.Is(err, ErrCorrupt) {
		test.Fatalf("Read error = %v, want %v", err, ErrCorrupt)
	}
	if !strings.Contains(err.Error(), path) {
		test.Errorf("Read error %q does not name the file", err)
	}
}

func TestReadReturnsPermissionError(test *testing.T) {
	if os.Geteuid() == 0 {
		test.Skip("root ignores file permissions")
	}
	path := Path(test.TempDir())
	if err := Write(path, []byte(`{"version":1,"marks":[]}`)); err != nil {
		test.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		test.Fatal(err)
	}
	if _, err := Read(path); !errors.Is(err, os.ErrPermission) {
		test.Errorf("Read error = %v, want a permission error", err)
	}
}

func TestWriteReturnsPermissionError(test *testing.T) {
	if os.Geteuid() == 0 {
		test.Skip("root ignores file permissions")
	}
	directory := test.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "strig"), 0o500); err != nil {
		test.Fatal(err)
	}
	if err := Write(Path(directory), []byte("x")); !errors.Is(err, os.ErrPermission) {
		test.Errorf("Write error = %v, want a permission error", err)
	}
}
