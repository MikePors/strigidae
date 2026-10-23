package pullrequestrows

import (
	"testing"
	"time"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

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
	withRepository := New(theme.Default(), time.Now, true)
	withoutRepository := New(theme.Default(), time.Now, false)

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
