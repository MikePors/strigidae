package diff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func load(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseMulti(t *testing.T) {
	d, err := Parse(load(t, "multi.diff"))
	if err != nil {
		t.Fatal(err)
	}
	type summary struct {
		Old, New string
		Status   Status
		Binary   bool
		Hunks    int
	}
	var got []summary
	for _, f := range d.Files {
		got = append(got, summary{f.OldPath, f.NewPath, f.Status, f.Binary, len(f.Hunks)})
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
		t.Fatalf("files (-want +got):\n%s", diff)
	}
}

func TestHunksAndLines(t *testing.T) {
	d, err := Parse(load(t, "multi.diff"))
	if err != nil {
		t.Fatal(err)
	}
	h := d.Files[0].Hunks
	if h[1].Section != "func helper() {" {
		t.Errorf("section = %q", h[1].Section)
	}
	if got := h[1].Header(); got != "@@ -20,4 +21,4 @@ func helper() {" {
		t.Errorf("Header() = %q", got)
	}
	wantLines := []Line{
		{Context, 20, 21, "\ta := 1"},
		{Delete, 21, 0, "\tb := 2"},
		{Add, 0, 22, "\tb := 3"},
		{Context, 22, 23, "\t_ = a + b"},
		{Context, 23, 24, "}"},
	}
	if diff := cmp.Diff(wantLines, h[1].Lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
	// Hunk 1: blank context line keeps its (empty) text and numbers.
	if l := h[0].Lines[1]; l.Kind != Context || l.Text != "" || l.Old != 2 || l.New != 2 {
		t.Errorf("blank context line = %+v", l)
	}
	if l := h[0].Lines[3]; l.Kind != Add || l.New != 4 || l.Text != `import "os"` {
		t.Errorf("added line = %+v", l)
	}
}

func TestNoNewline(t *testing.T) {
	d, _ := Parse(load(t, "multi.diff"))
	lines := d.File("nonl.txt").Hunks[0].Lines
	want := []Line{
		{Context, 1, 1, "keep"},
		{Delete, 2, 0, "old end"},
		{NoNewline, 0, 0, "No newline at end of file"},
		{Add, 0, 2, "new end"},
		{NoNewline, 0, 0, "No newline at end of file"},
	}
	if diff := cmp.Diff(want, lines); diff != "" {
		t.Errorf("lines (-want +got):\n%s", diff)
	}
}

func TestMetadata(t *testing.T) {
	d, _ := Parse(load(t, "multi.diff"))
	if f := d.File("lib/util.go"); f == nil || f.Similarity != 80 {
		t.Errorf("rename similarity: %+v", f)
	}
	if f := d.File("new.txt"); f == nil || f.Similarity != 100 || len(f.Hunks) != 0 {
		t.Errorf("pure rename: %+v", f)
	}
	f := d.File("run.sh")
	if f == nil || !f.ModeChanged() || f.OldMode != "100644" || f.NewMode != "100755" {
		t.Errorf("mode change: %+v", f)
	}
	if f := d.File("added.go"); f.NewMode != "100644" || f.ModeChanged() {
		t.Errorf("added mode: %+v", f)
	}
}

func TestFileLookup(t *testing.T) {
	d, _ := Parse(load(t, "multi.diff"))
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
	for _, tt := range tests {
		f := d.File(tt.path)
		switch {
		case tt.want == "-" && f != nil:
			t.Errorf("File(%q) = %+v, want nil", tt.path, f)
		case tt.want != "-" && f == nil:
			t.Errorf("File(%q) = nil", tt.path)
		case f != nil && f.NewPath != tt.want:
			t.Errorf("File(%q).NewPath = %q, want %q", tt.path, f.NewPath, tt.want)
		}
	}
	if p := d.File("gone.go").Path(); p != "gone.go" {
		t.Errorf("Path() of deleted = %q", p)
	}
}

func TestCRLF(t *testing.T) {
	for _, name := range []string{"crlf.diff", "crlf_body.diff"} {
		t.Run(name, func(t *testing.T) {
			d, err := Parse(load(t, name))
			if err != nil {
				t.Fatal(err)
			}
			f := d.File("win.txt")
			if f == nil {
				t.Fatal("file not found")
			}
			want := []Line{
				{Context, 1, 1, "one"},
				{Delete, 2, 0, "two"},
				{Add, 0, 2, "three"},
			}
			if diff := cmp.Diff(want, f.Hunks[0].Lines); diff != "" {
				t.Errorf("lines (-want +got):\n%s", diff)
			}
		})
	}
}

func TestEmptyAndGarbage(t *testing.T) {
	d, err := Parse("")
	if err != nil || len(d.Files) != 0 {
		t.Errorf("empty: %v, %d files", err, len(d.Files))
	}
	d, err = Parse(load(t, "empty.diff"))
	if err != nil || len(d.Files) != 0 {
		t.Errorf("empty fixture: %v, %d files", err, len(d.Files))
	}
	d, err = Parse("this is not a diff\nat all\n")
	var pe *ParseError
	if !errors.As(err, &pe) || d == nil {
		t.Errorf("garbage: err = %v, diff = %v", err, d)
	}
}

func TestTruncated(t *testing.T) {
	d, err := Parse(load(t, "truncated.diff"))
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *ParseError", err)
	}
	f := d.File("x.go")
	if f == nil || len(f.Hunks) != 1 || len(f.Hunks[0].Lines) != 2 {
		t.Errorf("best-effort result lost: %+v", f)
	}
}

func TestMalformedHunkRecovers(t *testing.T) {
	in := "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1,3 +1,3 @@\n x\nGARBAGE\ndiff --git a/b b/b\n--- a/b\n+++ b/b\n@@ -1 +1 @@\n-p\n+q\n"
	d, err := Parse(in)
	if err == nil {
		t.Error("want error for short hunk")
	}
	if len(d.Files) != 2 || len(d.Files[1].Hunks) != 1 || len(d.Files[1].Hunks[0].Lines) != 2 {
		t.Errorf("did not recover for second file: %+v", d.Files)
	}
}

func TestQuotedAndSpacedPaths(t *testing.T) {
	in := "diff --git \"a/caf\\303\\251.txt\" \"b/caf\\303\\251.txt\"\nnew file mode 100644\n--- /dev/null\n+++ \"b/caf\\303\\251.txt\"\n@@ -0,0 +1 @@\n+x\n"
	d, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Files[0].NewPath; got != "café.txt" {
		t.Errorf("NewPath = %q", got)
	}
}

func TestStatusAndKindStrings(t *testing.T) {
	if Renamed.String() != "renamed" || Delete.String() != "delete" || Status(99).String() != "unknown" {
		t.Error("String methods")
	}
}

func TestQuotedPathsWithTrailingTab(t *testing.T) {
	d, err := Parse(load(t, "quoted.diff"))
	if err != nil {
		t.Fatal(err)
	}
	type paths struct{ Old, New string }
	var got []paths
	for _, f := range d.Files {
		got = append(got, paths{f.OldPath, f.NewPath})
	}
	want := []paths{
		{"héllo wörld.txt", "héllo wörld.txt"},
		{"plain space.txt", "plain space.txt"},
		{"tab\there.txt", "new \"q\".txt"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("paths (-want +got):\n%s", diff)
	}
	for _, p := range []string{"héllo wörld.txt", "plain space.txt", "new \"q\".txt", "tab\there.txt"} {
		if d.File(p) == nil {
			t.Errorf("File(%q) = nil", p)
		}
	}
	if len(d.Files[0].Hunks) != 1 {
		t.Error("hunk lost")
	}
}

func TestBadPatchPathKeepsGitHeaderPath(t *testing.T) {
	in := "diff --git a/x.txt b/x.txt\n--- \"a/bad\\q\"\n+++ \"b/bad\\q\"\n@@ -1 +1 @@\n-a\n+b\n"
	d, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if f := d.Files[0]; f.OldPath != "x.txt" || f.NewPath != "x.txt" {
		t.Errorf("paths = %q, %q", f.OldPath, f.NewPath)
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"multi.diff", "crlf.diff", "truncated.diff"} {
		b, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(b))
	}
	f.Add("@@ -1 +1 @@\n")
	f.Add("diff --git \"a\n@@ -99999999999999999999 +1 @@\n")
	f.Fuzz(func(t *testing.T, in string) {
		d, _ := Parse(in)
		if d == nil {
			t.Fatal("nil diff")
		}
	})
}
