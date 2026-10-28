package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/diffview"
)

func TestPerFileDiff(test *testing.T) {
	_, built, err := perFileDiff([]github.FilePatch{
		{Path: "a.go", Change: github.FileModified, Additions: 1, Patch: "@@ -1 +1,2 @@\n a\n+b"},
		{Path: "new.go", Change: github.FileAdded, Additions: 1, Patch: "@@ -0,0 +1 @@\n+x"},
		{Path: "old.go", Change: github.FileDeleted, Deletions: 1, Patch: "@@ -1 +0,0 @@\n-x"},
		{Path: "after.md", PreviousPath: "before.md", Change: github.FileRenamed},
		{Path: "copy.md", PreviousPath: "source.md", Change: github.FileCopied, Additions: 3},
		{Path: "logo.png", Change: github.FileAdded},
		{Path: "huge.json", Change: github.FileModified, Additions: 90000},
		{Path: "mode.sh", Change: github.FileChanged, Patch: "@@ -1 +1 @@\n-a\n+b"},
	})
	if err != nil {
		test.Fatal(err)
	}
	type summary struct {
		OldPath, NewPath string
		Status           diff.Status
		Hunks            int
		PatchMissing     bool
	}
	var got []summary
	for _, file := range built.Files {
		got = append(got, summary{file.OldPath, file.NewPath, file.Status, len(file.Hunks), file.PatchMissing})
	}
	want := []summary{
		{"a.go", "a.go", diff.Modified, 1, false},
		{"", "new.go", diff.Added, 1, false},
		{"old.go", "", diff.Deleted, 1, false},
		{"before.md", "after.md", diff.Renamed, 0, false}, // a pure rename has nothing to patch
		{"source.md", "copy.md", diff.Copied, 0, true},
		{"", "logo.png", diff.Added, 0, true},
		{"huge.json", "huge.json", diff.Modified, 0, true},
		{"mode.sh", "mode.sh", diff.Modified, 1, false},
	}
	if difference := cmp.Diff(want, got); difference != "" {
		test.Errorf("mismatch (-want +got):\n%s", difference)
	}
	if built.File("before.md") != built.Files[3] {
		test.Error("a renamed file should be found under its old path too")
	}
}

func TestPerFileDiffParseErrorAndRaw(test *testing.T) {
	patches := []github.FilePatch{
		{Path: "good.go", Change: github.FileModified, Patch: "@@ -1 +1 @@\n-a\n+b"},
		{Path: "bad.go", Change: github.FileModified, Patch: "@@ -1,3 +1,3 @@\n a"},
	}
	raw, built, err := perFileDiff(patches)
	if err == nil || !strings.Contains(err.Error(), "bad.go") {
		test.Errorf("err = %v, want a parse error naming bad.go", err)
	}
	if len(built.Files) != 2 || len(built.File("good.go").Hunks) != 1 {
		test.Error("the files that parsed should still be there")
	}
	patches[0].Patch = "@@ -1 +1 @@\n-a\n+c"
	if changed, _, _ := perFileDiff(patches); changed == raw {
		test.Error("Raw should change when a patch does")
	}
	// A rename with no patch is a pure move or PatchMissing by its line counts.
	moved := []github.FilePatch{{Path: "b", PreviousPath: "a", Change: github.FileRenamed}}
	pureRaw, _, _ := perFileDiff(moved)
	moved[0].Additions = 5
	if changed, _, _ := perFileDiff(moved); changed == pureRaw {
		test.Error("Raw should change when the line counts do")
	}
}

// fixturePatches turns the test diff into the file list GitHub would give
// for it, so a diff built file by file can be compared with the whole one.
// README.md is in the pull request's files but not the diff: it comes back
// with no patch, as a file too large on its own would.
func fixturePatches(test *testing.T) []github.FilePatch {
	test.Helper()
	raw := readDiff(test)
	parsed, err := diff.Parse(raw)
	if err != nil {
		test.Fatal(err)
	}
	sections := strings.Split(raw, "diff --git ")[1:]
	changes := map[diff.Status]github.FileChangeType{
		diff.Modified: github.FileModified, diff.Added: github.FileAdded, diff.Deleted: github.FileDeleted,
		diff.Renamed: github.FileRenamed, diff.Copied: github.FileCopied,
	}
	var patches []github.FilePatch
	for i, file := range parsed.Files {
		patch := github.FilePatch{Path: file.Path(), Change: changes[file.Status]}
		if file.Status == diff.Renamed {
			patch.PreviousPath = file.OldPath
		}
		if start := strings.Index(sections[i], "@@"); start >= 0 {
			patch.Patch = strings.TrimSuffix(sections[i][start:], "\n")
		}
		patches = append(patches, patch)
	}
	return append(patches, github.FilePatch{Path: "README.md", Change: github.FileModified, Additions: 2, Deletions: 1})
}

func perFileHarness(test *testing.T, width, height int) *harness {
	test.Helper()
	perFile := newHarness(test, width, height)
	detail := testDetail()
	detail.HeadSHA = "cccc3333"
	perFile.client.SetPullRequest(detail)
	perFile.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
	perFile.client.SetFilePatches(testRepository, 7, fixturePatches(test))
	perFile.client.SetReviewThreads(testRepository, 7, testThreads())
	perFile.run(perFile.view.Init())
	return perFile
}

func TestLargeDiffLoadsFileByFile(test *testing.T) {
	perFile := perFileHarness(test, 100, 30)
	if !slices.Contains(perFile.messages, tea.Msg(userinterface.InfoMessage{Text: perFileNotice})) {
		test.Errorf("status messages = %#v, want %q", perFile.messages, perFileNotice)
	}
	if perFile.view.diff.Err != nil || perFile.view.diff.Parsed == nil {
		test.Fatalf("diff state = %+v, want a parsed diff and no error", perFile.view.diff)
	}
	// Inline comments and reviews check the head the diff was loaded at.
	if perFile.view.diffHead != "cccc3333" {
		test.Errorf("diff head = %q, want the detail's head", perFile.view.diffHead)
	}

	// Each file with a patch looks as it does in the whole diff, threads and
	// all, and the usual keys move through it.
	whole := threadHarness(test, 100, 30)
	steps := [][]string{{"}"}, {"l", "]", "j", ")"}, {"h", "}"}, {"}"}, {"}"}, {"}"}}
	for i, keys := range steps {
		whole.press(keys...)
		perFile.press(keys...)
		if path := perFile.view.selectedPath(); path == "README.md" {
			break // the file with no patch, checked below
		}
		if got, want := ansi.Strip(perFile.render()), ansi.Strip(whole.render()); got != want {
			test.Fatalf("step %d (%v): file by file differs from the whole diff:\n got:\n%s\nwant:\n%s", i, keys, got, want)
		}
	}
	if path := perFile.view.selectedPath(); path != "README.md" {
		test.Fatalf("should have reached README.md, at %q", path)
	}
	out := ansi.Strip(perFile.render())
	for _, line := range strings.Split(diffview.TooLargeMessage(), "\n") {
		if !strings.Contains(out, line) {
			test.Errorf("a file without a patch should show %q:\n%s", line, out)
		}
	}
	if !strings.Contains(out, "Typo.") {
		test.Errorf("a file without a patch should still list its thread:\n%s", out)
	}
}

func TestLargeDiffFileListFails(test *testing.T) {
	harness := newHarness(test, 100, 24)
	harness.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
	harness.client.SetError(fake.MethodListFilePatches, errors.New("connection reset"))
	harness.run(harness.view.Init())
	if !errors.Is(harness.view.diff.Err, github.ErrDiffTooLarge) {
		test.Errorf("diff error = %v, want the too-large error", harness.view.diff.Err)
	}
	if err := harness.lastErr(); err == nil || !strings.Contains(err.Error(), "connection reset") {
		test.Errorf("status error = %v, want the file list's failure", err)
	}
	harness.press("}")
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "too large to load") {
		test.Errorf("the pane should say the diff is too large:\n%s", out)
	}
}
