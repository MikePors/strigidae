package pullrequestrows

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestIndentedDraftKeepsItsStyle(test *testing.T) {
	activeTheme := theme.Default()
	pullRequests := []github.PullRequest{{Number: 7, Title: "Second step", Author: "alice", Draft: true}}
	table := New(activeTheme, time.Now, Options{})
	layout := table.Layout(pullRequests, 60)

	row := table.IndentedRow(layout, pullRequests[0], 2, false, false, 60)
	if want := "    " + activeTheme.PullRequestList.Draft.Render("[draft] "); !strings.Contains(row, want) {
		test.Errorf("row %q lacks the indent followed by the styled draft marker %q", row, want)
	}
	if plain := ansi.Strip(row); !strings.Contains(plain, "#7      [draft] Second step") {
		test.Errorf("plain row %q: title not indented by four columns", plain)
	}
}

func TestShortRepository(test *testing.T) {
	tests := []struct {
		text  string
		width int
		want  string
	}{
		{"acme/widgets", 12, "acme/widgets"},
		{"acme/widgets", 10, "…/widgets"},
		{"acme/widgets", 9, "…/widgets"},
		{"acme/widgets", 6, "…/wid…"},
		{"kubernetes/kubernetes", 10, "…/kuberne…"},
		{"octo-org/really-long-repo-name", 10, "…/really-…"},
		{"octo-org/really-long-repository-name", 24, "…/really-long-repositor…"},
		{"acme/widgets", 2, "a…"},
		{"widgets", 4, "wid…"},
	}
	for _, testCase := range tests {
		if got := shortRepository(testCase.text, testCase.width); got != testCase.want {
			test.Errorf("shortRepository(%q, %d) = %q, want %q", testCase.text, testCase.width, got, testCase.want)
		}
	}
}

func TestNarrowLayoutKeepsTheTitle(test *testing.T) {
	pullRequests := []github.PullRequest{{
		Repository: github.Repository{Owner: "acme", Name: "widgets"},
		Number:     12, Title: "A title", Author: "octo-dev",
	}}
	withRepository := New(theme.Default(), time.Now, Options{Repository: true})
	withoutRepository := New(theme.Default(), time.Now, Options{})

	tests := []struct {
		width          int
		wantRepository int // 0 means dropped
		wantAge        bool
	}{
		{80, 12, true},
		{37, 10, true}, // shrunk to its floor
		{36, 0, true},  // dropped
		{30, 0, true},
		{20, 0, false}, // even the age goes
	}
	for _, testCase := range tests {
		layout := withRepository.Layout(pullRequests, testCase.width-MarkerColumns)
		if layout.repository != testCase.wantRepository || (layout.age > 0) != testCase.wantAge {
			test.Errorf("width %d: repository %d age %d, want repository %d, age shown %v", testCase.width, layout.repository, layout.age, testCase.wantRepository, testCase.wantAge)
		}
		if testCase.width >= 30 && layout.title < minimumTitleColumns {
			test.Errorf("width %d: title only %d columns", testCase.width, layout.title)
		}
	}
	// A list without a repository column never loses its age column.
	if layout := withoutRepository.Layout(pullRequests, 20-MarkerColumns); layout.age == 0 {
		test.Error("the repository list dropped the age column")
	}
}

func TestBranchColumnsDropHeadFirstThenBase(test *testing.T) {
	pullRequests := []github.PullRequest{{
		Number: 12, Title: "A title", Author: "octo-dev",
		HeadBranch: "feature/a-branch-name-that-is-far-too-long", BaseBranch: "release/a-base-name-that-is-too-long",
	}}
	table := New(theme.Default(), time.Now, Options{Branches: true})

	tests := []struct {
		width                          int
		wantHead, wantBase, wantAuthor int
	}{
		{120, maximumHead, maximumBase, 8},
		{70, 0, maximumBase, 8},
		{50, 0, 0, 8},
		{30, 0, 0, 0}, // then the existing order
	}
	for _, testCase := range tests {
		layout := table.Layout(pullRequests, testCase.width-MarkerColumns)
		if layout.head != testCase.wantHead || layout.base != testCase.wantBase || layout.author != testCase.wantAuthor {
			test.Errorf("width %d: head %d base %d author %d, want %d %d %d", testCase.width, layout.head, layout.base, layout.author, testCase.wantHead, testCase.wantBase, testCase.wantAuthor)
		}
	}
	if layout := New(theme.Default(), time.Now, Options{}).Layout(pullRequests, 120); layout.head != 0 || layout.base != 0 {
		test.Error("a table without the option shows the branch columns")
	}
}

func TestBranchCellsAreTruncatedAndCleaned(test *testing.T) {
	pullRequests := []github.PullRequest{{
		Number: 1, Title: "T", HeadBranch: "feature/\x07a-branch-name-that-is-far-too-long\u202e",
		BaseBranch: "base\x1b]52;c;aGk=\x07\x1b[2J\u202e",
	}}
	table := New(theme.Default(), time.Now, Options{Branches: true})
	layout := table.Layout(pullRequests, 120)
	row := ansi.Strip(table.Row(layout, pullRequests[0], false, false, 120))
	if !strings.Contains(row, "feature/ a-branch-name-…") {
		test.Errorf("head not truncated to %d columns with an ellipsis: %q", maximumHead, row)
	}
	if strings.ContainsAny(row, "\x1b\x07\u202e") || !strings.Contains(row, "base") {
		test.Errorf("hostile branch not cleaned: %q", row)
	}
}

func TestMarkColumn(test *testing.T) {
	pullRequests := []github.PullRequest{
		{Number: 12, Title: "A title", Author: "octo-dev", HeadBranch: "feature", BaseBranch: "main"},
		{Number: 13, Title: "Another", Author: "octo-dev", HeadBranch: "fix", BaseBranch: "main"},
	}
	marks := map[int]string{13: "waiting on author, then more"}
	table := New(theme.Default(), time.Now, Options{Branches: true, Mark: func(pullRequest github.PullRequest) string { return marks[pullRequest.Number] }})
	unmarked := New(theme.Default(), time.Now, Options{Branches: true})

	if layout := unmarked.Layout(pullRequests, 100); layout.mark != 0 {
		test.Errorf("mark column of %d without a Mark function", layout.mark)
	}
	layout := table.Layout(pullRequests, 100)
	if layout.mark != maximumMark {
		test.Fatalf("mark column is %d wide, want %d", layout.mark, maximumMark)
	}
	if header := ansi.Strip(table.Header(layout, 100)); !strings.Contains(header, "MARK") {
		test.Errorf("header %q lacks MARK", header)
	}
	if row := ansi.Strip(table.Row(layout, pullRequests[1], false, false, 100)); !strings.Contains(row, "waiting on auth…") {
		test.Errorf("row %q lacks the truncated mark", row)
	}
	if row := ansi.Strip(table.Row(layout, pullRequests[0], false, false, 100)); strings.Contains(row, "…") {
		test.Errorf("unmarked row %q shows a mark", row)
	}

	// The mark goes before anything else on a narrow terminal.
	wide := unmarked.Layout(pullRequests, 100)
	for width := 100; width > 20; width-- {
		layout := table.Layout(pullRequests, width)
		if layout.mark == 0 {
			plain := unmarked.Layout(pullRequests, width)
			if layout != plain {
				test.Errorf("width %d: mark dropped but the rest differs: %+v, want %+v", width, layout, plain)
			}
			break
		}
		if layout.head != wide.head || layout.base != wide.base {
			test.Errorf("width %d: head %d base %d dropped before the mark", width, layout.head, layout.base)
		}
	}
}

func TestMarkIsCleanedBeforeItIsShown(test *testing.T) {
	pullRequests := []github.PullRequest{{Number: 1, Title: "T", Author: "a"}}
	table := New(theme.Default(), time.Now, Options{Mark: func(github.PullRequest) string { return "ok\x1b]52;c;x\x07\u202e" }})
	layout := table.Layout(pullRequests, 80)
	for _, row := range []string{table.Row(layout, pullRequests[0], false, false, 80), table.Row(layout, pullRequests[0], true, false, 80)} {
		if strings.ContainsAny(ansi.Strip(row), "\x1b\x07\u202e") {
			test.Errorf("row %q holds a control or bidirectional character", row)
		}
	}
}
