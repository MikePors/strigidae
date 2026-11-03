package localthreads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/plugins"
)

// wait returns when the flush ends, or fails the test.
func wait(test *testing.T, finished <-chan struct{}) {
	test.Helper()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		test.Fatal("the flush did not end")
	}
}

// writeSaved puts threads in the file of key, as an earlier run would have.
func writeSaved(test *testing.T, files *Files, key Key, threads []Thread) {
	test.Helper()
	data, err := Encode(threads)
	if err != nil {
		test.Fatal(err)
	}
	if err := Write(pathOf(test, files, key), data); err != nil {
		test.Fatal(err)
	}
}

// The flush does not depend on any write command having run.
func TestFlushWritesWhatNoCommandWrote(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("lost", baseTime))
	files.Changed(widgetKey) // a write command was made but never ran
	files.BeginWrite(widgetKey)
	wait(test, files.Flush(&store))
	if diff := cmp.Diff([]string{"lost"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
}

// A change waiting for the write in flight is written by the flush, though the
// write's end never reaches the caller.
func TestFlushWritesTheChangesWaitingForAWrite(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("first", baseTime))
	files.Changed(widgetKey)
	path, generation, _ := files.BeginWrite(widgetKey)
	inFlight := ToFile(store.ForRequest(widgetKey))
	store.Add(asked("second", baseTime))
	if got := files.Changed(widgetKey); got != Wait {
		test.Fatalf("second change = %v, want Wait", got)
	}
	finished := files.Flush(&store)
	if err := files.Write(widgetKey, path, generation, inFlight); err != nil { // the write ends meanwhile
		test.Fatal(err)
	}
	wait(test, finished)
	if diff := cmp.Diff([]string{"first", "second"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file after the flush (-want +got):\n%s", diff)
	}
}

func TestFlushDoesNotWriteWhenNothingWaits(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("why?", baseTime))
	files.Changed(widgetKey)
	writeNow(test, files, &store, widgetKey)
	path := pathOf(test, files, widgetKey)
	before, err := os.Stat(path)
	if err != nil {
		test.Fatal(err)
	}
	wait(test, files.Flush(&store))
	after, err := os.Stat(path)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		test.Errorf("the flush rewrote an up-to-date file: %v", err)
	}
}

func TestFlushWithNothingChangedHasNothingToDo(test *testing.T) {
	files, _ := newFiles(test)
	var store Store
	wait(test, files.Flush(&store))
	if entries, err := os.ReadDir(files.Directory); err != nil || len(entries) != 0 {
		test.Errorf("directory holds %v (%v)", entries, err)
	}
}

// A pull request whose saved threads were never read is read by the flush and
// merged, so the saved ones are not lost.
func TestFlushKeepsSavedThreadsThatWereNeverRead(test *testing.T) {
	files, _ := newFiles(test)
	writeSaved(test, files, widgetKey, []Thread{{Identifier: "local-1", Question: "old", State: StateAnswered, Asked: time.Unix(1, 0)}})
	var store Store
	store.Add(asked("new", baseTime))
	files.Changed(widgetKey) // the read never runs
	wait(test, files.Flush(&store))
	if diff := cmp.Diff([]string{"old", "new"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
}

// The merge in the flush follows the same rules as a load: the limits hold,
// identifiers do not clash.
func TestFlushMergesUnderTheStoresLimits(test *testing.T) {
	files, _ := newFiles(test)
	writeSaved(test, files, widgetKey, []Thread{
		{Identifier: "local-1", Question: "old1", State: StateAnswered, Asked: time.Unix(1, 0)},
		{Identifier: "local-2", Question: "old2", State: StateAnswered, Asked: time.Unix(2, 0)},
	})
	store := Store{Limit: 2}
	store.Add(asked("new", baseTime))
	files.Changed(widgetKey)
	wait(test, files.Flush(&store))
	file, err := Read(pathOf(test, files, widgetKey))
	if err != nil {
		test.Fatal(err)
	}
	var got []string
	for _, thread := range file.Threads {
		got = append(got, thread.Question+" "+thread.Identifier)
	}
	// The saved local-1 clashes with the one asked this run, so it is renumbered
	// and, being the oldest, is the one the limit drops.
	if diff := cmp.Diff([]string{"old2 local-2", "new local-1"}, got); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
}

// A file that can't be read when the flush merges is set aside and replaced.
func TestFlushSetsAsideAnUnreadableFile(test *testing.T) {
	files, logs := newFiles(test)
	path := pathOf(test, files, widgetKey)
	data := []byte("{secret-not-json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		test.Fatal(err)
	}
	var store Store
	store.Add(asked("new", baseTime))
	files.Changed(widgetKey)
	wait(test, files.Flush(&store))
	if aside, err := os.ReadFile(path + UnreadableSuffix); err != nil || string(aside) != string(data) {
		test.Errorf("unreadable copy missing or changed: %v", err)
	}
	if diff := cmp.Diff([]string{"new"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
	if !strings.Contains(logs.String(), "can't be decoded") || strings.Contains(logs.String(), "secret") {
		test.Errorf("log = %s", logs)
	}
}

// A stuck write holds the lock. The flush must hand the caller its channel at
// once, and finish when the write lets go.
func TestFlushDoesNotBlockTheCallerWhileAWriteIsStuck(test *testing.T) {
	files, _ := openedFiles(test)
	var store Store
	store.Add(asked("why?", baseTime))
	files.Changed(widgetKey)
	release := files.HoldForTest()
	defer release()
	returned := make(chan (<-chan struct{}))
	go func() { returned <- files.Flush(&store) }()
	var finished <-chan struct{}
	select {
	case finished = <-returned: // returns although the lock is held
	case <-time.After(5 * time.Second):
		test.Fatal("Flush blocked while the lock was held")
	}
	select {
	case <-finished:
		test.Fatal("the flush ended while the lock was held")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	wait(test, files.Flushed())
	if diff := cmp.Diff([]string{"why?"}, savedQuestions(test, files, widgetKey)); diff != "" {
		test.Errorf("file (-want +got):\n%s", diff)
	}
}

// A pull request whose latest copy was already written is left alone: the
// flush does not even read its file.
func TestFlushSkipsACopyAlreadyWritten(test *testing.T) {
	files, logs := newFiles(test)
	var store Store
	store.Add(asked("why?", baseTime))
	files.Changed(widgetKey)
	path, generation, _ := files.BeginWrite(widgetKey)
	if err := files.Write(widgetKey, path, generation, ToFile(store.ForRequest(widgetKey))); err != nil {
		test.Fatal(err)
	}
	// Something else now makes the file unusable; a flush that read it would say so.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		test.Fatal(err)
	}
	wait(test, files.Flush(&store))
	if logs.Len() != 0 {
		test.Errorf("the flush read a file whose latest copy was written: %s", logs)
	}
	if kept, err := os.ReadFile(path); err != nil || string(kept) != "{not json" {
		test.Errorf("file = %q (%v), want it untouched", kept, err)
	}
}

// One answer is at most what a plugin may print, which must stay far below
// what a file may hold, so the newest thread always fits. Answers hold code,
// so characters JSON could escape are the worst ordinary case.
func TestAFullSizeAnswerFitsInAFile(test *testing.T) {
	text := strings.Repeat("<a & b>\n", plugins.MaximumOutputSize/8)
	size := FileOverhead + ThreadSize(Thread{Question: "q", Text: text, State: StateAnswered})
	if size*2 > MaxFileSize {
		test.Errorf("a full-size answer takes %d bytes of a %d byte file", size, MaxFileSize)
	}
}
