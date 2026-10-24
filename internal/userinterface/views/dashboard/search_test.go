package dashboard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// The first tab has rows 0 (acme/widgets#12), 1 (acme/gadgets#7) and
// 2 (rivka/dotfiles#3); the second has Rivka's refactor (acme/widgets#40).

func promptResult(view *View, text string, state userinterface.PromptState) []tea.Msg {
	_, command := view.Update(userinterface.PromptResultMessage{Prefix: "/", Text: text, State: state})
	return messages(command)
}

func TestSlashAsksForThePrompt(test *testing.T) {
	view, _ := loadedView(test)
	got := messages(press(view, "/"))
	if len(got) != 1 {
		test.Fatalf("/ produced %v", got)
	}
	if message, ok := got[0].(userinterface.PromptMessage); !ok || message.Prefix != "/" {
		test.Errorf("/ produced %v", got[0])
	}
}

func TestTypingMovesTheCursorAndEscRestoresIt(test *testing.T) {
	view, _ := loadedView(test)
	press(view, "j") // row 1
	press(view, "/")
	promptResult(view, "dotfiles", userinterface.PromptChanged)
	if got := selectedKey(test, view); got != "rivka/dotfiles#3" {
		test.Errorf("selected %s while typing", got)
	}
	promptResult(view, "zzz", userinterface.PromptChanged)
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("selected %s with no match, want the start row", got)
	}
	promptResult(view, "dotfiles", userinterface.PromptChanged)
	promptResult(view, "dotfiles", userinterface.PromptCancelled)
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("esc left the cursor on %s", got)
	}
	if view.tabs[0].search.Active() {
		test.Error("a cancelled search is still active")
	}
}

func TestEnterKeepsMarksAndEscClearsThem(test *testing.T) {
	view, _ := loadedView(test)
	press(view, "/")
	promptResult(view, "acme", userinterface.PromptChanged)
	promptResult(view, "acme", userinterface.PromptSubmitted)
	marker := theme.Default().SearchMarker.Render(search.Glyph)
	if !strings.Contains(view.Render(80, 16), marker) {
		test.Error("matching rows are not marked")
	}
	press(view, "esc")
	if strings.Contains(view.Render(80, 16), search.Glyph) {
		test.Error("marks survive esc")
	}
}

func TestNextPreviousAndCounts(test *testing.T) {
	view, _ := loadedView(test)
	press(view, "/")
	promptResult(view, "acme", userinterface.PromptChanged)
	promptResult(view, "acme", userinterface.PromptSubmitted) // on row 0
	press(view, "n")
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("n: %s", got)
	}
	press(view, "n") // wraps
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("n wrap: %s", got)
	}
	press(view, "N") // wraps back
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("N wrap: %s", got)
	}
	view.Update(keys.Message{Sequence: "n", Count: 3})
	if got := selectedKey(test, view); got != "acme/widgets#12" {
		test.Errorf("3n: %s, want three steps round the two matches", got)
	}
}

func TestNoMatchSaysSoAndKeepsTheCursor(test *testing.T) {
	view, _ := loadedView(test)
	press(view, "j")
	press(view, "/")
	promptResult(view, "zzz", userinterface.PromptChanged)
	var info []string
	for _, message := range promptResult(view, "zzz", userinterface.PromptSubmitted) {
		if text, ok := message.(userinterface.InfoMessage); ok {
			info = append(info, text.Text)
		}
	}
	if len(info) != 1 || info[0] != "no match: zzz" {
		test.Errorf("info = %q", info)
	}
	if got := selectedKey(test, view); got != "acme/gadgets#7" {
		test.Errorf("cursor moved to %s", got)
	}
}

func TestEachTabKeepsItsOwnSearch(test *testing.T) {
	view, _ := loadedView(test)
	press(view, "/")
	promptResult(view, "tidy", userinterface.PromptChanged)
	promptResult(view, "tidy", userinterface.PromptSubmitted)
	settle(view, press(view, "g t"))
	if view.tabs[1].search.Active() {
		test.Error("the second tab inherited the search")
	}
	if strings.Contains(ansi.Strip(view.Render(80, 16)), search.Glyph) {
		test.Error("the second tab shows marks")
	}
	press(view, "/")
	promptResult(view, "refactor", userinterface.PromptChanged)
	promptResult(view, "refactor", userinterface.PromptSubmitted)
	settle(view, press(view, "g T"))
	if view.tabs[0].search.Query() != "tidy" || view.tabs[1].search.Query() != "refactor" {
		test.Errorf("queries = %q and %q", view.tabs[0].search.Query(), view.tabs[1].search.Query())
	}
	if !view.tabs[0].search.Marked(2) {
		test.Error("the first tab lost its marks")
	}
}

func TestRefreshRecomputesTheMarks(test *testing.T) {
	view, client := loadedView(test)
	press(view, "/")
	promptResult(view, "tidy", userinterface.PromptChanged)
	promptResult(view, "tidy", userinterface.PromptSubmitted)
	if !view.tabs[0].search.Marked(2) {
		test.Fatal("setup: row 2 is not marked")
	}
	tabs := testTabs(test)
	renamed := pullRequest("rivka/dotfiles", 3, "Something else", 0)
	review := pullRequest("acme/widgets", 12, "Tidy the cache", 0)
	client.SetSearchResults(searchQuery(tabs[0].Sections[0]), []github.PullRequest{review})
	client.SetSearchResults(searchQuery(tabs[0].Sections[1]), []github.PullRequest{renamed})
	settle(view, press(view, "R"))
	if view.tabs[0].search.Query() != "tidy" || !view.tabs[0].search.Marked(0) || view.tabs[0].search.Marked(1) {
		test.Error("marks were not recomputed after the refresh")
	}
}

func TestHelpListsTheSearchKeys(test *testing.T) {
	view, _ := loadedView(test)
	var got []string
	for _, group := range view.FullHelp() {
		for _, binding := range group {
			got = append(got, binding.Help().Key)
		}
	}
	if !strings.Contains(strings.Join(got, " "), "/ n N") {
		test.Errorf("help keys = %q", got)
	}
}
