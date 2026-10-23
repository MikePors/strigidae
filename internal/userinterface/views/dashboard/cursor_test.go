package dashboard

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
)

func TestHostileRefreshErrorIsNeutralised(test *testing.T) {
	view, client := loadedView(test)
	hostile := "boom\x1b[2J\x07" + string(rune(0x202e)) + "\u009b"
	client.SetError(fake.MethodSearchPullRequests, errors.New(hostile))
	settle(view, press(view, "R"))
	raw := view.Render(100, 16) // not stripped: styling escapes are expected, hostile ones are not
	if !strings.Contains(raw, "refresh failed: boom") {
		test.Fatalf("the failed refresh is not shown: %q", raw)
	}
	for _, bad := range []string{"\x1b[2J", "\x07", string(rune(0x202e)), "\u009b"} {
		if strings.Contains(raw, bad) {
			test.Errorf("render contains %q from the error", bad)
		}
	}
}

func TestFirstSectionEmptyKeepsItsHeadingAtTheTop(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	client.SetSearchResults(searchQuery(tabs[0].Sections[1]), []github.PullRequest{pullRequest("o/a", 1, "A1", 0), pullRequest("o/a", 2, "A2", 0)})
	client.SetSearchResults(searchQuery(tabs[0].Sections[2]), []github.PullRequest{pullRequest("o/b", 3, "B1", 0), pullRequest("o/b", 4, "B2", 0)})
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "G")
	view.Render(80, 7)
	press(view, "g g")
	out := ansi.Strip(view.Render(80, 7))
	if !strings.Contains(out, "Review requested") || !strings.Contains(out, "No pull requests") {
		test.Errorf("the empty first section scrolled off:\n%s", out)
	}
}

func TestCursorStaysWhereItIsShownWhenItsSectionEmptiesAndRefills(test *testing.T) {
	tabs := testTabs(test)
	client := fake.New()
	seed(test, client, tabs)
	view := newView(client, tabs, nil)
	settle(view, view.Init())
	press(view, "g g") // anchor on the first row of section 0
	first := searchQuery(tabs[0].Sections[0])
	refill := []github.PullRequest{pullRequest("acme/widgets", 12, "Cache the parsed diff", time.Hour)}

	client.SetSearchResults(first, nil)
	settle(view, press(view, "R"))
	shown := selectedKey(test, view)
	if shown != "rivka/dotfiles#3" {
		test.Fatalf("with section 0 empty the cursor shows %s, want rivka/dotfiles#3", shown)
	}
	client.SetSearchResults(first, refill)
	settle(view, press(view, "R"))
	if got := selectedKey(test, view); got != shown {
		test.Errorf("after section 0 refilled the cursor jumped to %s, want it to stay on %s", got, shown)
	}
}
