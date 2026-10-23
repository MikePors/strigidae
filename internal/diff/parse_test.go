package diff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func load(test *testing.T, name string) string {
	test.Helper()
	bytes, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		test.Fatal(err)
	}
	return string(bytes)
}

func TestParseMultiple(test *testing.T) {
	diff, err := Parse(load(test, "multi.diff"))
	if err != nil {
		test.Fatal(err)
	}
	type summary struct {
		Old, New string
		Status   Status
		Binary   bool
		Hunks    int
	}
	var got []summary
	for _, file := range diff.Files {
		got = append(got, summary{file.OldPath, file.NewPath, file.Status, file.Binary, len(file.Hunks)})
	}
	want := []summary{
		{"main.go", "main.go", Modified, false, 2},
		{"old.txt", "new.txt", Renamed, false, 0},
		{"util.go", "lib/util.go", Renamed, false, 1},
		{"logo.png", "logo.png", Modified, true, 0},
		{"", "added.go", Added, false, 1},
		{"gone.go", "", Deleted, false, 1},
		{"run.sh", "run.sh", Modified, false, 0},
		{"nonl.txt", "nonl.txt", Modified, false, 1},
		{"with space.txt", "with space.txt", Modified, false, 1},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Fatalf("files (-want +got):\n%s", diff)
	}
}

func TestHunksAndLines(test *testing.T) {
	diff, err := Parse(load(test, "multi.diff"))
	if err != nil {
		test.Fatal(err)
	}
	hunk := diff.Files[0].Hunks
	if hunk[1].Section != "func helper() {" {
		test.Errorf("section = %q", hunk[1].Section)
	}
	if got := hunk[1].Header(); got != "@@ -20,4 +21,4 @@ func helper() {" {
		test.Errorf("Header() = %q", got)
	}
	wantLines := []Line{
		{Context, 20, 21, "\ta := 1"},
		{Delete, 21, 0, "\tb := 2"},
		{Add, 0, 22, "\tb := 3"},
		{Context, 22, 23, "\t_ = a + b"},
		{Context, 23, 24, "}"},
	}
	if diff := cmp.Diff(wantLines, hunk[1].Lines); diff != "" {
		test.Errorf("lines (-want +got):\n%s", diff)
	}
	// Hunk 1: blank context line keeps its (empty) text and numbers.
	if line := hunk[0].Lines[1]; line.Kind != Context || line.Text != "" || line.Old != 2 || line.New != 2 {
		test.Errorf("blank context line = %+v", line)
	}
	if line := hunk[0].Lines[3]; line.Kind != Add || line.New != 4 || line.Text != `import "os"` {
		test.Errorf("added line = %+v", line)
	}
}

func TestNoNewline(test *testing.T) {
	diff, _ := Parse(load(test, "multi.diff"))
	lines := diff.File("nonl.txt").Hunks[0].Lines
	want := []Line{
		{Context, 1, 1, "keep"},
		{Delete, 2, 0, "old end"},
		{NoNewline, 0, 0, "No newline at end of file"},
		{Add, 0, 2, "new end"},
		{NoNewline, 0, 0, "No newline at end of file"},
	}
	if diff := cmp.Diff(want, lines); diff != "" {
		test.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestMetadata(test *testing.T) {
	diff, _ := Parse(load(test, "multi.diff"))
	if file := diff.File("lib/util.go"); file == nil || file.Similarity != 80 {
		test.Errorf("rename similarity: %+v", file)
	}
	if file := diff.File("new.txt"); file == nil || file.Similarity != 100 || len(file.Hunks) != 0 {
		test.Errorf("pure rename: %+v", file)
	}
	file := diff.File("run.sh")
	if file == nil || !file.ModeChanged() || file.OldMode != "100644" || file.NewMode != "100755" {
		test.Errorf("mode change: %+v", file)
	}
	if file := diff.File("added.go"); file.NewMode != "100644" || file.ModeChanged() {
		test.Errorf("added mode: %+v", file)
	}
}

func TestFileLookup(test *testing.T) {
	diff, _ := Parse(load(test, "multi.diff"))
	tests := []struct {
		path string
		want string // expected NewPath ("" for deleted), or "-" for nil
	}{
		{"main.go", "main.go"},
		{"lib/util.go", "lib/util.go"},
		{"util.go", "lib/util.go"}, // old path of a rename
		{"gone.go", ""},            // deleted: found by old path
		{"missing.go", "-"},
		{"", "-"},
	}
	for _, testCase := range tests {
		file := diff.File(testCase.path)
		switch {
		case testCase.want == "-" && file != nil:
			test.Errorf("File(%q) = %+v, want nil", testCase.path, file)
		case testCase.want != "-" && file == nil:
			test.Errorf("File(%q) = nil", testCase.path)
		case file != nil && file.NewPath != testCase.want:
			test.Errorf("File(%q).NewPath = %q, want %q", testCase.path, file.NewPath, testCase.want)
		}
	}
	if path := diff.File("gone.go").Path(); path != "gone.go" {
		test.Errorf("Path() of deleted = %q", path)
	}
}

func TestCRLF(test *testing.T) {
	for _, name := range []string{"crlf.diff", "crlf_body.diff"} {
		test.Run(name, func(test *testing.T) {
			diff, err := Parse(load(test, name))
			if err != nil {
				test.Fatal(err)
			}
			file := diff.File("win.txt")
			if file == nil {
				test.Fatal("file not found")
			}
			want := []Line{
				{Context, 1, 1, "one"},
				{Delete, 2, 0, "two"},
				{Add, 0, 2, "three"},
			}
			if diff := cmp.Diff(want, file.Hunks[0].Lines); diff != "" {
				test.Errorf("lines (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEmptyAndGarbage(test *testing.T) {
	diff, err := Parse("")
	if err != nil || len(diff.Files) != 0 {
		test.Errorf("empty: %v, %d files", err, len(diff.Files))
	}
	diff, err = Parse(load(test, "empty.diff"))
	if err != nil || len(diff.Files) != 0 {
		test.Errorf("empty fixture: %v, %d files", err, len(diff.Files))
	}
	diff, err = Parse("this is not a diff\nat all\n")
	var parseError *ParseError
	if !errors.As(err, &parseError) || diff == nil {
		test.Errorf("garbage: err = %v, diff = %v", err, diff)
	}
}

func TestTruncated(test *testing.T) {
	diff, err := Parse(load(test, "truncated.diff"))
	var parseError *ParseError
	if !errors.As(err, &parseError) {
		test.Fatalf("err = %v, want *ParseError", err)
	}
	file := diff.File("x.go")
	if file == nil || len(file.Hunks) != 1 || len(file.Hunks[0].Lines) != 2 {
		test.Errorf("best-effort result lost: %+v", file)
	}
}

func TestMalformedHunkRecovers(test *testing.T) {
	in := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,3 +1,3 @@\n x\nGARBAGE\ndiff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -1 +1 @@\n-p\n+q\n"
	diff, err := Parse(in)
	if err == nil {
		test.Error("want error for short hunk")
	}
	if len(diff.Files) != 2 || len(diff.Files[1].Hunks) != 1 || len(diff.Files[1].Hunks[0].Lines) != 2 {
		test.Errorf("did not recover for second file: %+v", diff.Files)
	}
}

func TestQuotedAndSpacedPaths(test *testing.T) {
	in := "diff --git \"a/caf\\303\\251.txt\" \"b/caf\\303\\251.txt\"\nnew file mode 100644\n--- /dev/null\n+++ \"b/caf\\303\\251.txt\"\n@@ -0,0 +1 @@\n+x\n"
	diff, err := Parse(in)
	if err != nil {
		test.Fatal(err)
	}
	if got := diff.Files[0].NewPath; got != "café.txt" {
		test.Errorf("NewPath = %q", got)
	}
}

func TestStatusAndKindStrings(test *testing.T) {
	if Renamed.String() != "renamed" || Delete.String() != "delete" || Status(99).String() != "unknown" {
		test.Error("String methods")
	}
}

func TestQuotedPathsWithTrailingTab(test *testing.T) {
	diff, err := Parse(load(test, "quoted.diff"))
	if err != nil {
		test.Fatal(err)
	}
	type paths struct{ Old, New string }
	var got []paths
	for _, file := range diff.Files {
		got = append(got, paths{file.OldPath, file.NewPath})
	}
	want := []paths{
		{"héllo wörld.txt", "héllo wörld.txt"},
		{"plain space.txt", "plain space.txt"},
		{"tab\there.txt", "new \"q\".txt"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		test.Fatalf("paths (-want +got):\n%s", diff)
	}
	for _, path := range []string{"héllo wörld.txt", "plain space.txt", "new \"q\".txt", "tab\there.txt"} {
		if diff.File(path) == nil {
			test.Errorf("File(%q) = nil", path)
		}
	}
	if len(diff.Files[0].Hunks) != 1 {
		test.Error("hunk lost")
	}
}

func TestBadPatchPathKeepsGitHeaderPath(test *testing.T) {
	in := "diff --git a/x.txt b/x.txt\n--- \"a/bad\\q\"\n+++ \"b/bad\\q\"\n@@ -1 +1 @@\n-a\n+b\n"
	diff, err := Parse(in)
	if err != nil {
		test.Fatal(err)
	}
	if file := diff.Files[0]; file.OldPath != "x.txt" || file.NewPath != "x.txt" {
		test.Errorf("paths = %q, %q", file.OldPath, file.NewPath)
	}
}

func TestLinesBeyondCounts(test *testing.T) {
	const head = "diff --git a/f b/f\n--- a/f\n+++ b/f\n"
	tests := []struct {
		name    string
		body    string
		wantErr bool
		lines   int
	}{
		{"exact", "@@ -1 +1 @@\n-a\n+b\n", false, 2},
		{"extra add and delete", "@@ -1 +1 @@\n-a\n+b\n+c\n-d\n", true, 2},
		{"extra context", "@@ -1 +1 @@\n-a\n+b\n tail\n", true, 2},
		{"extra after marker", "@@ -1 +1 @@\n-a\n+b\n\\ No newline at end of file\n+c\n", true, 3},
		{"trailing blank line is fine", "@@ -1 +1 @@\n-a\n+b\n\n", false, 2},
		{"second hunk after exact first", "@@ -1 +1 @@\n-a\n+b\n@@ -5 +5 @@\n-c\n+d\n", false, 2},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			diff, err := Parse(head + testCase.body)
			if (err != nil) != testCase.wantErr {
				test.Fatalf("err = %v, wantErr %v", err, testCase.wantErr)
			}
			if got := len(diff.Files[0].Hunks[0].Lines); got != testCase.lines {
				test.Errorf("first hunk has %d lines, want %d", got, testCase.lines)
			}
		})
	}
}

func TestCRLFBlankContextLine(test *testing.T) {
	in := "diff --git a/w b/w\r\n--- a/w\r\n+++ b/w\r\n@@ -1,3 +1,3 @@\r\n one\r\n\r\n-two\r\n+three\r\n"
	diff, err := Parse(in)
	if err != nil {
		test.Fatal(err)
	}
	want := []Line{
		{Context, 1, 1, "one"},
		{Context, 2, 2, ""},
		{Delete, 3, 0, "two"},
		{Add, 0, 3, "three"},
	}
	if difference := cmp.Diff(want, diff.Files[0].Hunks[0].Lines); difference != "" {
		test.Errorf("lines (-want +got):\n%s", difference)
	}
}

func FuzzParse(fuzzer *testing.F) {
	for _, name := range []string{"multi.diff", "crlf.diff", "truncated.diff"} {
		bytes, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			fuzzer.Fatal(err)
		}
		fuzzer.Add(string(bytes))
	}
	fuzzer.Add("@@ -1 +1 @@\n")
	fuzzer.Add("diff --git \"a\n@@ -99999999999999999999 +1 @@\n")
	fuzzer.Fuzz(func(test *testing.T, in string) {
		diff, _ := Parse(in)
		if diff == nil {
			test.Fatal("nil diff")
		}
	})
}
