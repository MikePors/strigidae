package workspace

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/search"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
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
	var prompts []userinterface.PromptMessage
	_, command := harness.view.Update(keys.Message{Sequence: "/"})
	for _, message := range messages(command) {
		if prompt, ok := message.(userinterface.PromptMessage); ok {
			prompts = append(prompts, prompt)
		}
	}
	if len(prompts) != 1 || prompts[0].Prefix != "/" {
		test.Errorf("prompts = %v", prompts)
	}
	harness.press("l")
	_, command = harness.view.Update(keys.Message{Sequence: "/"})
	if len(messages(command)) != 0 {
		test.Error("/ in the right pane opened a prompt")
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
	if !strings.HasPrefix(ansi.Strip(rows[2]), "> ") {
		test.Errorf("selected row = %q", ansi.Strip(rows[2]))
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
