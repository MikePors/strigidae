package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// The Files pane rows are 0 Overview, 1 client.go, 2 retry.go, 3 legacy.go,
// 4 docs/design/.../retry-policy-and-backoff.md and 5 README.md. "retry"
// matches rows 2 and 4.

func (harness *harness) searchFor(text string, state userinterface.PromptState) {
	harness.test.Helper()
	harness.send(userinterface.PromptResultMessage{Prefix: "/", Text: text, State: state})
}

func (harness *harness) startSearch(text string) {
	harness.test.Helper()
	harness.press("/")
	harness.searchFor(text, userinterface.PromptChanged)
}

func TestSlashInTheFilesPaneAsksForThePrompt(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("/")
	if prefix, ok := harness.view.PromptRequest(); !ok || prefix != "/" {
		test.Errorf("after / the request is %q, %v", prefix, ok)
	}
	harness.press("l", "/")
	if _, ok := harness.view.PromptRequest(); ok {
		test.Error("/ in the right pane asked for a prompt")
	}
}

func TestRestoreFollowsTheFileWhenFilesChange(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j", "j") // retry.go
	harness.press("/")
	harness.searchFor("zzz", userinterface.PromptChanged)
	detail := testDetail()
	detail.Files = append(detail.Files[:0:0], detail.Files[1:]...) // client.go is gone
	harness.send(pullRequestLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: harness.view.sequence, detail: detail})
	harness.searchFor("zzz", userinterface.PromptCancelled)
	if got := harness.view.selectedPath(); got != "internal/sync/retry.go" {
		test.Errorf("esc restored %q", got)
	}
}

func TestSubmitJumpsToTheFirstMatchAtOrAfterTheOriginInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j", "j", "j") // legacy.go
	harness.press("/")
	harness.searchFor("retry", userinterface.PromptSubmitted)
	if got := harness.view.selection(); got != 4 {
		test.Errorf("selection = %d, want 4", got)
	}
}

func TestHelpSaysTheSearchKeysSearchTheFilesPane(test *testing.T) {
	var descriptions []string
	for _, group := range loaded(test, 100, 30).view.searchHelp() {
		descriptions = append(descriptions, group.Help().Desc)
	}
	for _, description := range descriptions {
		if !strings.Contains(description, "Files pane") {
			test.Errorf("help %q does not say what is searched", description)
		}
	}
}

func TestTypingMovesTheFilesCursorAndEscRestoresIt(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j") // row 1
	harness.startSearch("retry")
	if got := harness.view.selection(); got != 2 {
		test.Errorf("selection = %d, want 2", got)
	}
	harness.searchFor("zzz", userinterface.PromptChanged)
	if got := harness.view.selection(); got != 1 {
		test.Errorf("selection = %d with no match, want 1", got)
	}
	harness.searchFor("retry", userinterface.PromptChanged)
	harness.searchFor("retry", userinterface.PromptCancelled)
	if got := harness.view.selection(); got != 1 {
		test.Errorf("esc left the selection on %d", got)
	}
	if harness.view.search.Active() {
		test.Error("a cancelled search is still active")
	}
}

func TestNextPreviousAndCountsInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted) // row 2
	harness.press("n")
	if got := harness.view.selection(); got != 4 {
		test.Errorf("n: %d, want 4", got)
	}
	harness.press("n")
	if got := harness.view.selection(); got != 2 {
		test.Errorf("n wrap: %d, want 2", got)
	}
	harness.press("N")
	if got := harness.view.selection(); got != 4 {
		test.Errorf("N wrap: %d, want 4", got)
	}
	harness.pressCounted("n", 3)
	if got := harness.view.selection(); got != 2 {
		test.Errorf("3n: %d, want 2", got)
	}
	if got := harness.view.rightTitle(200); got != "internal/sync/retry.go" {
		test.Errorf("the right pane shows %q", got)
	}
}

func TestNoMatchInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("j")
	harness.startSearch("zzz")
	harness.searchFor("zzz", userinterface.PromptSubmitted)
	found := false
	for _, message := range harness.messages {
		if info, ok := message.(userinterface.InfoMessage); ok && info.Text == "no match: zzz" {
			found = true
		}
	}
	if !found {
		test.Errorf("no match message missing: %v", harness.messages)
	}
	if got := harness.view.selection(); got != 1 {
		test.Errorf("selection = %d, want 1", got)
	}
}

func TestMarksAndEscInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("README")
	harness.searchFor("README", userinterface.PromptSubmitted) // selects row 5
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted) // selects row 2
	marker := theme.New(true).SearchMarker.Render(search.Glyph)
	rows := harness.view.fileRows(40, 10)
	if strings.Contains(rows[0], search.Glyph) || strings.Contains(rows[1], search.Glyph) {
		test.Error("rows that do not match carry the marker")
	}
	if !strings.HasPrefix(rows[4], marker) {
		test.Errorf("row 4 = %q, want the styled marker first", rows[4])
	}
	if !strings.HasPrefix(ansi.Strip(rows[2]), search.SelectedGlyph) {
		test.Errorf("selected matching row = %q, want the cursor and the match glyph", ansi.Strip(rows[2]))
	}
	if strings.Contains(rows[2], marker) {
		test.Error("the selected row draws the marker in the accent colour")
	}
	harness.press("esc")
	if strings.Contains(ansi.Strip(strings.Join(harness.view.fileRows(40, 10), "\n")), search.Glyph) {
		test.Error("marks survive esc")
	}
}

func TestRefreshRecomputesTheFilesMarks(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted)
	detail := testDetail()
	detail.Files = detail.Files[:2] // client.go, retry.go
	harness.send(pullRequestLoadedMessage{owner: harness.view, repository: testRepository, number: 7, sequence: harness.view.sequence, detail: detail})
	if !harness.view.search.Marked(2) || harness.view.search.Marked(4) {
		test.Error("marks were not recomputed")
	}
}

func TestHelpListsTheSearchKeys(test *testing.T) {
	var got []string
	for _, group := range loaded(test, 100, 30).view.FullHelp() {
		for _, binding := range group {
			got = append(got, binding.Help().Key)
		}
	}
	if !strings.Contains(strings.Join(got, " "), "/ n N") {
		test.Errorf("help keys = %q", got)
	}
}

func TestEnterOnAnEmptyPromptRepeatsTheLastSearchInTheFilesPane(test *testing.T) {
	harness := loaded(test, 100, 30)
	harness.press("/")
	harness.searchFor("", userinterface.PromptSubmitted)
	if harness.view.search.Active() {
		test.Fatal("an empty prompt with no previous search started one")
	}
	harness.startSearch("retry")
	harness.searchFor("retry", userinterface.PromptSubmitted)
	harness.press("esc", "g g")
	harness.press("/")
	harness.searchFor("", userinterface.PromptSubmitted)
	if harness.view.search.Query() != "retry" || !harness.view.search.Marked(4) {
		test.Errorf("query = %q", harness.view.search.Query())
	}
	if got := harness.view.selection(); got != 2 {
		test.Errorf("selection = %d, want 2", got)
	}
}
