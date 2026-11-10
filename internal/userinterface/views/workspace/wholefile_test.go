package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// wholeFileDiff has a modified Go file whose hunk starts inside a block
// comment opened above it, then an added file.
const wholeFileDiff = `diff --git a/x.go b/x.go
index 1111111..2222222 100644
--- a/x.go
+++ b/x.go
@@ -4,3 +4,3 @@
 still comment
 closed */
-var a = 1
+var b = 2
diff --git a/new.go b/new.go
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new.go
@@ -0,0 +1 @@
+package x
`

// fakeBlobs is a BlobReader holding blobs in memory.
type fakeBlobs struct {
	texts map[string]string
	err   error
	// timeOuts is how many reads fail with context.DeadlineExceeded first.
	timeOuts int
	reads    []string // "number:identifier", in order
}

func (blobs *fakeBlobs) ReadBlob(_ context.Context, number int, identifier string) ([]byte, error) {
	blobs.reads = append(blobs.reads, fmt.Sprintf("%d:%s", number, identifier))
	if blobs.err != nil {
		return nil, blobs.err
	}
	if blobs.timeOuts > 0 {
		blobs.timeOuts--
		return nil, fmt.Errorf("read blob: %w", context.DeadlineExceeded)
	}
	text, ok := blobs.texts[identifier]
	if !ok {
		return nil, fmt.Errorf("blob %s not found", identifier)
	}
	return []byte(text), nil
}

// spansPane is the real diff pane, recording each side it is given once.
type spansPane struct {
	*diffPane
	given map[string][][]diffview.Span // by identifier
	order []string
}

func (pane *spansPane) SetFileSpans(side github.DiffSide, identifier string, lines [][]diffview.Span) {
	if _, seen := pane.given[identifier]; !seen {
		pane.given[identifier] = lines
		pane.order = append(pane.order, string(side)+":"+identifier)
	}
	pane.diffPane.SetFileSpans(side, identifier, lines)
}

func newWholeFileHarness(test *testing.T, blobs *fakeBlobs) (*harness, *spansPane) {
	test.Helper()
	return newWholeFileHarnessFor(test, blobs, nil, nil)
}

// flakyBlobClient fails its first failures BlobTexts calls with a timeout; a
// negative count fails them all.
type flakyBlobClient struct {
	*fake.Client
	failures int
}

func (client *flakyBlobClient) BlobTexts(ctx context.Context, repository github.Repository, identifiers []string) ([]github.BlobText, error) {
	if client.failures != 0 {
		client.failures--
		return nil, fmt.Errorf("read blobs: %w", context.DeadlineExceeded)
	}
	return client.Client.BlobTexts(ctx, repository, identifiers)
}

// newWholeFileHarnessFor opens the whole-file pull request with a checkout
// reader (nil: none; the API only) and a client wrapper (nil: the fake as
// it is). Patches, if any, make the diff load file by file.
func newWholeFileHarnessFor(test *testing.T, blobs *fakeBlobs, wrap func(*fake.Client) github.Client, patches []github.FilePatch) (*harness, *spansPane) {
	test.Helper()
	client := fake.New()
	detail := testDetail()
	detail.Files = []github.ChangedFile{
		{Path: "x.go", Change: github.FileModified, Additions: 1, Deletions: 1},
		{Path: "new.go", Change: github.FileAdded, Additions: 1},
	}
	client.SetPullRequest(detail)
	if patches != nil {
		client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
		client.SetFilePatches(testRepository, 7, patches)
	} else {
		client.SetDiff(testRepository, 7, wholeFileDiff)
	}
	var used github.Client = client
	if wrap != nil {
		used = wrap(client)
	}
	var reader BlobReader // a nil *fakeBlobs must not become a non-nil interface
	if blobs != nil {
		reader = blobs
	}
	pane := &spansPane{diffPane: newDiffPane(theme.New(true), func() time.Time { return testNow }, false), given: map[string][][]diffview.Span{}}
	testHarness := &harness{test: test, client: client}
	testHarness.view = New(used, testRepository, 7,
		WithClock(func() time.Time { return testNow }),
		WithTheme(theme.New(true)),
		WithFilePane(pane),
		WithWholeFiles(NewWholeFiles(reader)),
	)
	testHarness.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	testHarness.run(testHarness.view.Init())
	return testHarness, pane
}

func bothSides() *fakeBlobs {
	return &fakeBlobs{texts: map[string]string{
		"1111111": "package x\n\n/* opened above\nstill comment\nclosed */\nvar a = 1\n",
		"2222222": "package x\n\n/* opened above\nstill comment\nclosed */\nvar b = 2\n",
	}}
}

// allComment reports whether every non-blank span is a comment.
func allComment(spans []diffview.Span) bool {
	for _, span := range spans {
		if strings.TrimSpace(span.Text) != "" && span.Token.Category() != chroma.Comment {
			return false
		}
	}
	return len(spans) > 0
}

func TestWholeFileColoursAHunkStartingInsideAComment(test *testing.T) {
	blobs := bothSides()
	testHarness, pane := newWholeFileHarness(test, blobs)
	testHarness.press("}") // x.go

	if diff := cmp.Diff([]string{"7:1111111", "7:2222222"}, blobs.reads); diff != "" {
		test.Errorf("reads (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"LEFT:1111111", "RIGHT:2222222"}, pane.order); diff != "" {
		test.Fatalf("sides given to the pane (-want +got):\n%s", diff)
	}
	for _, identifier := range []string{"1111111", "2222222"} {
		lines := pane.given[identifier]
		if !allComment(lines[3]) || !allComment(lines[4]) {
			test.Errorf("%s: lines 4 and 5 = %v, %v; want comments", identifier, lines[3], lines[4])
		}
	}

	// Another file and back: the sides come from the cache, not a new read.
	testHarness.press("}", "{")
	if len(blobs.reads) != 2 {
		test.Errorf("reads after reopening = %v, want the first two only", blobs.reads)
	}
}

func TestWholeFileResultForAFileNoLongerShownIsDropped(test *testing.T) {
	blobs := bothSides()
	testHarness, pane := newWholeFileHarness(test, blobs)
	// Open x.go but hold its reads back.
	view, pending := testHarness.view.Update(keys.Message{Sequence: "}"})
	testHarness.view = view.(*View)
	testHarness.press("}") // new.go is shown when the reads finish
	for _, message := range messages(pending) {
		testHarness.send(message)
	}
	if len(blobs.reads) != 2 {
		test.Fatalf("reads = %v, want both sides of x.go", blobs.reads)
	}
	if len(pane.order) != 0 {
		test.Errorf("sides given while another file is shown: %v", pane.order)
	}
}

func TestWholeFileFailedReadKeepsPerHunkColours(test *testing.T) {
	blobs := &fakeBlobs{err: errors.New("fetch failed")}
	testHarness, pane := newWholeFileHarness(test, blobs)
	testHarness.press("}")
	if len(pane.order) != 0 {
		test.Errorf("sides given after a failed read: %v", pane.order)
	}
	for _, message := range testHarness.messages {
		if _, ok := message.(userinterface.ErrorMessage); ok {
			test.Errorf("a failed read showed an error: %v", message)
		}
	}
	// It is not read again.
	testHarness.press("}", "{")
	if len(blobs.reads) != 2 {
		test.Errorf("reads = %v, want one per side", blobs.reads)
	}
	if strings.Contains(testHarness.render(), "fetch failed") {
		test.Error("the failure is on screen")
	}
}

func TestWholeFileTimedOutReadIsReadAgain(test *testing.T) {
	blobs := bothSides()
	blobs.timeOuts = 1
	// The API times out too: the side's last error is a timeout.
	wrap := func(client *fake.Client) github.Client { return &flakyBlobClient{Client: client, failures: 1} }
	testHarness, pane := newWholeFileHarnessFor(test, blobs, wrap, nil)
	testHarness.press("}")
	// Both sides are read together, then the one that timed out again.
	if diff := cmp.Diff([]string{"7:1111111", "7:2222222", "7:1111111"}, blobs.reads); diff != "" {
		test.Errorf("reads (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"RIGHT:2222222", "LEFT:1111111"}, pane.order); diff != "" {
		test.Errorf("sides given to the pane (-want +got):\n%s", diff)
	}
}

func TestWholeFileSideThatKeepsTimingOutIsReadTwice(test *testing.T) {
	blobs := bothSides()
	blobs.timeOuts = 100
	wrap := func(client *fake.Client) github.Client { return &flakyBlobClient{Client: client, failures: -1} }
	testHarness, pane := newWholeFileHarnessFor(test, blobs, wrap, nil)
	testHarness.press("}")
	if diff := cmp.Diff([]string{"7:1111111", "7:2222222", "7:1111111", "7:2222222"}, blobs.reads); diff != "" {
		test.Errorf("reads (-want +got):\n%s", diff)
	}
	// The failure is kept: another file and back reads nothing.
	testHarness.press("}", "{")
	if len(blobs.reads) != 4 {
		test.Errorf("reads after reopening = %v, want two per side", blobs.reads)
	}
	if len(pane.order) != 0 {
		test.Errorf("sides given after timeouts: %v", pane.order)
	}
}

// blobTextCalls returns the identifiers of each BlobTexts call made.
func blobTextCalls(client *fake.Client) [][]string {
	var asked [][]string
	for _, recorded := range client.Calls() {
		if recorded.Method == fake.MethodBlobTexts {
			asked = append(asked, recorded.ObjectIdentifiers)
		}
	}
	return asked
}

func TestWholeFileWithoutACheckoutReadsBothSidesInOneAPICall(test *testing.T) {
	testHarness, pane := newWholeFileHarnessFor(test, nil, nil, nil)
	testHarness.client.SetBlobText(testRepository, "1111111", "package x\n\n/* opened above\nstill comment\nclosed */\nvar a = 1\n")
	testHarness.client.SetBlobText(testRepository, "2222222", "package x\n\n/* opened above\nstill comment\nclosed */\nvar b = 2\n")
	testHarness.press("}")
	if diff := cmp.Diff([][]string{{"1111111", "2222222"}}, blobTextCalls(testHarness.client)); diff != "" {
		test.Errorf("BlobTexts calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"LEFT:1111111", "RIGHT:2222222"}, pane.order); diff != "" {
		test.Fatalf("sides given to the pane (-want +got):\n%s", diff)
	}
	for _, identifier := range []string{"1111111", "2222222"} {
		lines := pane.given[identifier]
		if !allComment(lines[3]) || !allComment(lines[4]) {
			test.Errorf("%s: lines 4 and 5 = %v, %v; want comments", identifier, lines[3], lines[4])
		}
	}
	// Reopening asks nothing more.
	testHarness.press("}", "{")
	if len(blobTextCalls(testHarness.client)) != 1 {
		test.Errorf("BlobTexts calls after reopening = %v", blobTextCalls(testHarness.client))
	}
}

func TestWholeFileOnlyTheSideTheCheckoutLacksGoesToTheAPI(test *testing.T) {
	blobs := bothSides()
	delete(blobs.texts, "1111111")
	testHarness, pane := newWholeFileHarnessFor(test, blobs, nil, nil)
	testHarness.client.SetBlobText(testRepository, "1111111", "package x\n\n/* opened above\nstill comment\nclosed */\nvar a = 1\n")
	testHarness.press("}")
	if diff := cmp.Diff([][]string{{"1111111"}}, blobTextCalls(testHarness.client)); diff != "" {
		test.Errorf("BlobTexts calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"LEFT:1111111", "RIGHT:2222222"}, pane.order); diff != "" {
		test.Errorf("sides given to the pane (-want +got):\n%s", diff)
	}
}

func TestWholeFileFromTheCheckoutAsksTheAPINothing(test *testing.T) {
	testHarness, pane := newWholeFileHarnessFor(test, bothSides(), nil, nil)
	testHarness.press("}")
	if calls := blobTextCalls(testHarness.client); len(calls) != 0 {
		test.Errorf("BlobTexts calls = %v, want none", calls)
	}
	if len(pane.order) != 2 {
		test.Errorf("sides given to the pane: %v", pane.order)
	}
}

func TestWholeFileAPIFailureKeepsPerHunkColours(test *testing.T) {
	testHarness, pane := newWholeFileHarnessFor(test, nil, nil, nil)
	testHarness.client.SetError(fake.MethodBlobTexts, errors.New("api down"))
	testHarness.press("}")
	if len(pane.order) != 0 {
		test.Errorf("sides given after a failed API call: %v", pane.order)
	}
	for _, message := range testHarness.messages {
		if _, ok := message.(userinterface.ErrorMessage); ok {
			test.Errorf("a failed read showed an error: %v", message)
		}
	}
	if strings.Contains(testHarness.render(), "api down") {
		test.Error("the failure is on screen")
	}
	// It is not asked again.
	testHarness.press("}", "{")
	if len(blobTextCalls(testHarness.client)) != 1 {
		test.Errorf("BlobTexts calls = %v, want one", blobTextCalls(testHarness.client))
	}
}

func TestWholeFileOfAFileByFileDiffColoursTheNewSide(test *testing.T) {
	patches := []github.FilePatch{
		{Path: "x.go", Change: github.FileModified, Patch: "@@ -4,3 +4,3 @@\n still comment\n closed */\n-var a = 1\n+var b = 2", ObjectIdentifier: "2222222"},
		{Path: "new.go", Change: github.FileAdded, Patch: "@@ -0,0 +1 @@\n+package x", ObjectIdentifier: "3333333"},
	}
	testHarness, pane := newWholeFileHarnessFor(test, nil, nil, patches)
	testHarness.client.SetBlobText(testRepository, "2222222", "package x\n\n/* opened above\nstill comment\nclosed */\nvar b = 2\n")
	testHarness.press("}")
	if diff := cmp.Diff([][]string{{"2222222"}}, blobTextCalls(testHarness.client)); diff != "" {
		test.Errorf("BlobTexts calls (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"RIGHT:2222222"}, pane.order); diff != "" {
		test.Fatalf("sides given to the pane (-want +got):\n%s", diff)
	}
	if lines := pane.given["2222222"]; !allComment(lines[3]) {
		test.Errorf("line 4 = %v, want a comment", lines[3])
	}
}

// TestWholeFilesStartAgainEmptyPastTheLimit crosses the limit twice, so a
// reset that forgot the byte count would keep one side at most afterwards.
func TestWholeFilesStartAgainEmptyPastTheLimit(test *testing.T) {
	side := [][]diffview.Span{{{Text: "x"}}}
	files := NewWholeFiles(nil)
	files.limit = 2 * spansSize(side)
	sides := []wholeFileKey{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}, {"e", "5"}, {"f", "6"}}
	for _, key := range sides {
		files.keep(key, side)
	}
	for i, key := range sides {
		_, kept := files.spans[key]
		if want := i >= 4; kept != want {
			test.Errorf("side %v kept = %v, want %v", key, kept, want)
		}
	}
}

func TestWholeFileAddedFileTriggersNoRead(test *testing.T) {
	blobs := bothSides()
	testHarness, _ := newWholeFileHarness(test, blobs)
	testHarness.view.selectEntry(2) // new.go, without passing x.go
	testHarness.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	if testHarness.view.paneFile.Path != "new.go" {
		test.Fatalf("shown file = %q", testHarness.view.paneFile.Path)
	}
	if len(blobs.reads) != 0 {
		test.Errorf("reads for an added file: %v", blobs.reads)
	}
}
