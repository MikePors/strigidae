package editor

import (
	"errors"
	"testing"
)

func TestParsePullRequestText(test *testing.T) {
	for _, testCase := range []struct {
		name      string
		text      string
		wantTitle string
		wantBody  string
		wantErr   error
	}{
		{"title and body", "Title\n\nBody\n", "Title", "Body", nil},
		{"blank lines in the body", "Title\n\nOne\n\n\nTwo\n\n", "Title", "One\n\n\nTwo", nil},
		{"empty body", "Title\n\n", "Title", "", nil},
		{"title only", "Title", "Title", "", nil},
		{"title is trimmed", "  Title \t\n\nBody", "Title", "Body", nil},
		{"crlf", "Title\r\n\r\nOne\r\n\r\nTwo\r\n", "Title", "One\n\nTwo", nil},
		{"leading blank line is an empty title", "\nTitle\n\nBody", "", "", ErrEmptyTitle},
		{"empty text", "", "", "", ErrEmptyTitle},
		{"blank title", "   \n\nBody", "", "", ErrEmptyTitle},
		{"no blank line after the title", "Title\nBody", "", "", ErrMissingBlankLine},
		{"body keeps leading blank lines after the separator", "Title\n\n\nBody", "Title", "\nBody", nil},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			title, body, err := ParsePullRequestText(testCase.text)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}
			if title != testCase.wantTitle || body != testCase.wantBody {
				test.Errorf("got (%q, %q), want (%q, %q)", title, body, testCase.wantTitle, testCase.wantBody)
			}
		})
	}
}

func TestPullRequestTextRoundTrip(test *testing.T) {
	for _, body := range []string{"", "Body", "One\n\nTwo\n\n\nThree", "\nstarts blank"} {
		title, got, err := ParsePullRequestText(FormatPullRequestText("Title", body))
		if err != nil || title != "Title" || got != body {
			test.Errorf("body %q came back as (%q, %q, %v)", body, title, got, err)
		}
	}
}
