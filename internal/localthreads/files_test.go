package localthreads

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
)

// newFiles is a Files over a fresh state directory, with its log.
func newFiles(test *testing.T) (*Files, *bytes.Buffer) {
	test.Helper()
	logs := &bytes.Buffer{}
	return &Files{Directory: test.TempDir(), Logger: slog.New(slog.NewTextHandler(logs, nil))}, logs
}

func pathOf(test *testing.T, files *Files, key Key) string {
	test.Helper()
	path, ok := files.Path(key)
	if !ok {
		test.Fatal("no file for the pull request")
	}
	return path
}

// savedQuestions is the questions in the file of key.
func savedQuestions(test *testing.T, files *Files, key Key) []string {
	test.Helper()
	file, err := Read(pathOf(test, files, key))
	if err != nil {
		test.Fatal(err)
	}
	var got []string
	for _, thread := range file.Threads {
		got = append(got, thread.Question)
	}
	return got
}

// writeNow runs the write a change leads to: begin, the command's work, end.
func writeNow(test *testing.T, files *Files, store *Store, key Key) {
	test.Helper()
	path, generation, ok := files.BeginWrite(key)
	if !ok {
		test.Fatal("BeginWrite found no file")
	}
	err := files.Write(key, path, generation, ToFile(store.ForRequest(key)))
	files.EndWrite(key, err)
	if err != nil {
		test.Fatal(err)
	}
}

// openedFiles is a Files whose pull request was opened and read, with
// nothing in it.
func openedFiles(test *testing.T) (*Files, *bytes.Buffer) {
	test.Helper()
	files, logs := newFiles(test)
	if _, ok := files.BeginRead(widgetKey); !ok {
		test.Fatal("BeginRead refused")
	}
	files.EndRead(widgetKey, nil, false)
	return files, logs
}

func TestPathKeepsNothingWithoutAUsableName(test *testing.T) {
	for _, testCase := range []struct {
		name      string
		directory string
		key       Key
		explained bool
	}{
		{"no state directory", "", widgetKey, false},
		{"empty host", "x", Key{Repository: widgets, Number: 7}, true},
		{"slash in host", "x", Key{Host: "a/b", Repository: widgets, Number: 7}, true},
		{"dot dot owner", "x", Key{Host: "github.com", Repository: github.Repository{Owner: "..", Name: "widgets"}, Number: 7}, true},
		{"slash in name", "x", Key{Host: "github.com", Repository: github.Repository{Owner: "acme", Name: "x/y"}, Number: 7}, true},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			files, logs := newFiles(test)
			if testCase.directory == "" {
				files.Directory = ""
			}
			for range 2 {
				if path, ok := files.Path(testCase.key); ok {
					test.Fatalf("Path = %q, want none", path)
				}
			}
			if _, ok := files.BeginRead(testCase.key); ok {
				test.Error("a read began for a pull request with no file")
			}
			if _, _, ok := files.BeginWrite(testCase.key); ok {
				test.Error("a write began for a pull request with no file")
			}
			wantLogged := 0
			if testCase.explained {
				wantLogged = 1
			}
			if got := strings.Count(logs.String(), "local threads are not kept"); got != wantLogged {
				test.Errorf("explained %d times, want %d:\n%s", got, wantLogged, logs)
			}
			if entries, err := os.ReadDir(files.Directory); testCase.explained && (err != nil || len(entries) != 0) {
				test.Errorf("state directory holds %v (%v), want nothing", entries, err)
			}
		})
	}
}

func TestReadsHappenOncePerRun(test *testing.T) {
	files, _ := newFiles(test)
	if _, ok := files.BeginRead(widgetKey); !ok {
		test.Fatal("the first read did not begin")
	}
	if _, ok := files.BeginRead(widgetKey); ok {
		test.Error("a second read began while the first was in flight")
	}
	files.EndRead(widgetKey, nil, false)
	if _, ok := files.BeginRead(widgetKey); ok {
		test.Error("a second read began after the first")
	}
}

// While the first read is in flight nothing is written over the file, and a
// change that comes meanwhile is written when the read ends.
func TestAChangeBeforeTheReadEndsWaitsForIt(test *testing.T) {
	files, _ := newFiles(test)
	if got := files.Changed(widgetKey); got != ReadFirst {
		test.Fatalf("a change before any read = %v, want ReadFirst", got)
	}
	if _, ok := files.BeginRead(widgetKey); !ok {
		test.Fatal("the read did not begin")
	}
	if got := files.Changed(widgetKey); got != Wait {
		test.Errorf("a change during the read = %v, want Wait", got)
	}
	if !files.EndRead(widgetKey, nil, false) {
		test.Error("the read ended with changes waiting and no write due")
	}
	if files.EndRead(widgetKey, nil, false) {
		test.Error("a write was due with nothing waiting")
	}
}

func TestEndReadSaysWhenAWriteIsDue(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		err     error
		changed bool
		want    bool
	}{
		{"nothing changed", nil, false, false},
		{"the merge changed the threads", nil, true, true},
		{"the file can't be used", ErrCorrupt, false, false},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			files, _ := newFiles(test)
			files.BeginRead(widgetKey)
			if got := files.EndRead(widgetKey, testCase.err, testCase.changed); got != testCase.want {
				test.Errorf("write due = %v, want %v", got, testCase.want)
			}
		})
	}
}

// A write command is held back and run by hand, to see what a change made
// while it was in flight does.
func TestWritesAreOneAtATimeAndInOrder(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("first", baseTime))
	if got := files.Changed(widgetKey); got != WriteNow {
		test.Fatalf("a change with nothing in flight = %v, want WriteNow", got)
	}
	path, generation, _ := files.BeginWrite(widgetKey)
	store.Add(asked("second", baseTime))
	if got := files.Changed(widgetKey); got != Wait {
		test.Errorf("a second change during the write = %v, want Wait", got)
	}
	store.Add(asked("third", baseTime))
	if got := files.Changed(widgetKey); got != Wait {
		test.Errorf("a third change during the write = %v, want Wait", got)
	}

	// The first write holds only what was there when it started.
	first := ToFile(store.ForRequest(widgetKey)[:1])
	if err := files.Write(widgetKey, path, generation, first); err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]string{"first"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Fatalf("file after the first write (-want +got):\n%s", diff)
	}
	// Its end asks for one write of the latest copy.
	if !files.EndWrite(widgetKey, nil) {
		test.Fatal("no write followed for the changes made meanwhile")
	}
	writeLatest := func() {
		path, generation, _ := files.BeginWrite(widgetKey)
		if err := files.Write(widgetKey, path, generation, ToFile(store.ForRequest(widgetKey))); err != nil {
			test.Fatal(err)
		}
	}
	writeLatest()
	if diff := cmp.Diff([]string{"first", "second", "third"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file after the last write (-want +got):\n%s", diff)
	}
	if files.EndWrite(widgetKey, nil) {
		test.Error("a write followed with nothing changed")
	}
	// Idle again: the next change writes at once.
	if got := files.Changed(widgetKey); got != WriteNow {
		test.Errorf("a change after the writes ended = %v, want WriteNow", got)
	}
}

// A copy made before a newer one was written never replaces it, whoever
// writes it.
func TestAnOlderCopyNeverLandsAfterANewerOne(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("first", baseTime))
	files.Changed(widgetKey)
	path, olderGeneration, _ := files.BeginWrite(widgetKey)
	older := ToFile(store.ForRequest(widgetKey))
	store.Add(asked("second", baseTime))
	files.Changed(widgetKey)
	_, newerGeneration, _ := files.BeginWrite(widgetKey)
	if newerGeneration <= olderGeneration {
		test.Fatalf("generations %d then %d: a change did not make a newer one", olderGeneration, newerGeneration)
	}
	if err := files.Write(widgetKey, path, newerGeneration, ToFile(store.ForRequest(widgetKey))); err != nil {
		test.Fatal(err)
	}
	if err := files.Write(widgetKey, path, olderGeneration, older); err != nil { // the older command runs late
		test.Fatal(err)
	}
	if diff := cmp.Diff([]string{"first", "second"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
	// The same generation again is not a newer copy either.
	if err := files.Write(widgetKey, path, newerGeneration, older); err != nil {
		test.Fatal(err)
	}
	if diff := cmp.Diff([]string{"first", "second"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file after the same generation again (-want +got):\n%s", diff)
	}
}

func TestEachChangeIsWritten(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	identifier := store.Add(asked("why?", baseTime))
	files.Changed(widgetKey)
	writeNow(test, files, &store, widgetKey)
	file, err := Read(pathOf(test, files, widgetKey))
	if err != nil || len(file.Threads) != 1 || file.Threads[0].State != StateWaiting || file.Threads[0].Question != "why?" {
		test.Fatalf("file after asking = %+v (%v)", file, err)
	}
	store.Finish(identifier, github.LocalAnswered, "because")
	files.Changed(widgetKey)
	writeNow(test, files, &store, widgetKey)
	file, err = Read(pathOf(test, files, widgetKey))
	if err != nil || len(file.Threads) != 1 || file.Threads[0].State != StateAnswered || file.Threads[0].Text != "because" {
		test.Errorf("file after the answer = %+v (%v)", file, err)
	}
}

func TestFilesThatCannotBeUsed(test *testing.T) {
	for _, testCase := range []struct {
		name    string
		content []byte
		logged  string
	}{
		{"corrupt", []byte("{secret-not-json"), "can't be decoded"},
		{"unknown version", []byte(`{"version":2,"threads":[{"question":"secret"}]}`), "unknown version"},
		{"too large", append([]byte(`{"version":1,"threads":[],"p":"`), bytes.Repeat([]byte("s"), MaxFileSize)...), "too large"},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			files, logs := newFiles(test)
			path, ok := files.BeginRead(widgetKey)
			if !ok {
				test.Fatal("BeginRead refused")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				test.Fatal(err)
			}
			if err := os.WriteFile(path, testCase.content, 0o600); err != nil {
				test.Fatal(err)
			}
			entries, err := Load(path, widgetKey)
			if err == nil || len(entries) != 0 {
				test.Fatalf("Load = %v, %v, want an error and nothing", entries, err)
			}
			files.EndRead(widgetKey, err, false)
			if !strings.Contains(logs.String(), testCase.logged) {
				test.Errorf("log lacks %q: %s", testCase.logged, logs)
			}
			if strings.Contains(logs.String(), "secret") {
				test.Errorf("log holds file content: %s", logs)
			}
			// Reading alone leaves the file where it is.
			if kept, err := os.ReadFile(path); err != nil || !bytes.Equal(kept, testCase.content) {
				test.Fatalf("file changed by reading: %v", err)
			}
			// The first write moves it aside.
			var store Store
			store.Add(asked("why?", baseTime))
			files.Changed(widgetKey)
			writeNow(test, files, &store, widgetKey)
			aside, err := os.ReadFile(path + UnreadableSuffix)
			if err != nil || !bytes.Equal(aside, testCase.content) {
				test.Errorf("unreadable copy missing or changed: %v", err)
			}
			if diff := cmp.Diff([]string{"why?"}, savedQuestions(test, files, widgetKey)); diff != "" {
				test.Errorf("new file (-want +got):\n%s", diff)
			}
			// A second write does not move the new file aside again.
			store.Add(asked("again?", baseTime))
			files.Changed(widgetKey)
			writeNow(test, files, &store, widgetKey)
			if diff := cmp.Diff([]string{"why?", "again?"}, savedQuestions(test, files, widgetKey)); diff != "" {
				test.Errorf("file after the second write (-want +got):\n%s", diff)
			}
			if aside, _ := os.ReadFile(path + UnreadableSuffix); !bytes.Equal(aside, testCase.content) {
				test.Error("the unreadable copy was replaced")
			}
			if _, err := os.Lstat(path + UnreadableSuffix + ".2"); err == nil {
				test.Error("the second write set the new file aside too")
			}
		})
	}
}

func TestFailedWriteIsLoggedWithoutText(test *testing.T) {
	files, logs := openedFiles(test)
	// A file where the directory should be makes every write fail.
	if err := os.WriteFile(filepath.Join(files.Directory, "strig"), []byte("x"), 0o600); err != nil {
		test.Fatal(err)
	}
	var store Store
	store.Add(asked("top secret question", baseTime))
	files.Changed(widgetKey)
	path, generation, _ := files.BeginWrite(widgetKey)
	err := files.Write(widgetKey, path, generation, ToFile(store.ForRequest(widgetKey)))
	if err == nil {
		test.Fatal("the write succeeded")
	}
	files.EndWrite(widgetKey, err)
	if !strings.Contains(logs.String(), "local threads not saved") {
		test.Errorf("failure not logged: %s", logs)
	}
	if strings.Contains(logs.String(), "secret") {
		test.Errorf("log holds thread text: %s", logs)
	}
	// A failure stops nothing: the next change writes at once.
	if got := files.Changed(widgetKey); got != WriteNow {
		test.Errorf("a change after a failed write = %v, want WriteNow", got)
	}
}
