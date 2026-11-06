package userinterface

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/golden"
)

// typeSearch opens the help search prompt and types query without entering it.
func typeSearch(model *Model, query string) {
	press(model, "/")
	for _, character := range query {
		press(model, string(character))
	}
}

// bottomRow is the status bar or prompt line, without styling.
func bottomRow(model *Model) string {
	lines := strings.Split(screen(model), "\n")
	return lines[len(lines)-1]
}

// markedRows counts the body rows of the screen that carry the search marker.
func markedRows(model *Model) int {
	return strings.Count(model.View().Content, model.theme.SearchMarker.Render("•"))
}

func TestHelpSlashOpensThePromptOnEveryPage(test *testing.T) {
	tests := []struct {
		name string
		open func(model *Model)
		want string
	}{
		{"keys", func(model *Model) { press(model, "?") }, "Global"},
		{"topics", func(model *Model) { enterLine(model, "help") }, "config"},
		{"config", func(model *Model) { enterLine(model, "help config") }, "wrapscan"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := sizedModel(80, 20)
			testCase.open(model)
			typeSearch(model, testCase.want)
			if got := bottomRow(model); !strings.HasPrefix(got, "/"+testCase.want) {
				test.Errorf("bottom row = %q, want the prompt /%s", got, testCase.want)
			}
			if markedRows(model) == 0 {
				test.Errorf("typing %q marked no row", testCase.want)
			}
			press(model, "enter")
			if !model.showHelp || !model.helpSearch.active() {
				test.Errorf("enter must keep the search in the open overlay")
			}
		})
	}
}

func TestHelpSearchMarksWholeRowsIgnoringCase(test *testing.T) {
	model := sizedModel(100, 30)
	enterLine(model, "help config")
	typeSearch(model, "RESOLVED")
	want := 0
	for _, line := range strings.Split(screen(model), "\n") {
		if strings.Contains(strings.ToLower(line), "resolved") && strings.Contains(line, "•") {
			want++
		}
	}
	if want == 0 || markedRows(model) != want {
		test.Errorf("marked %d rows, %d rows with the marker on screen", markedRows(model), want)
	}
}

func TestHelpSearchStepsAndScrolls(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	typeSearch(model, "action 5")
	press(model, "enter")
	first := model.helpSearch.current
	visible := func(row int) bool {
		return row >= model.helpOffset && row < model.helpOffset+helpBodyRows(11)
	}
	if !visible(first) {
		test.Fatalf("first match %d not in view at offset %d", first, model.helpOffset)
	}
	press(model, "n")
	second := model.helpSearch.current
	if second <= first || !visible(second) {
		test.Errorf("n went from %d to %d, offset %d", first, second, model.helpOffset)
	}
	press(model, "N")
	if model.helpSearch.current != first || !visible(first) {
		test.Errorf("N went to %d, want %d", model.helpSearch.current, first)
	}
}

func TestHelpSearchWrapscan(test *testing.T) {
	tests := []struct {
		name    string
		line    string
		wrapped bool
	}{
		{"wrapscan on", "set wrapscan", true},
		{"wrapscan off", "set nowrapscan", false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := helpModel(newLongHelpView(60), 60, 12)
			press(model, "esc")
			enterLine(model, testCase.line)
			press(model, "?")
			typeSearch(model, "action 59")
			press(model, "enter")
			last := model.helpSearch.current
			command := press(model, "n")
			if testCase.wrapped {
				if model.helpSearch.current != last || command != nil {
					test.Errorf("a single match wraps onto itself")
				}
				return
			}
			if command == nil {
				test.Fatalf("no message at the end with wrapscan off")
			}
			if message, ok := command().(InfoMessage); !ok || message.Text != "no more matches below" {
				test.Errorf("message = %#v", command())
			}
		})
	}
}

func TestHelpSearchWithoutHighlightMarksNothing(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "set nohlsearch")
	enterLine(model, "help config")
	typeSearch(model, "wrapscan")
	press(model, "enter")
	if markedRows(model) != 0 || strings.Contains(screen(model), "•") {
		test.Errorf("hlsearch off still marked rows")
	}
	if len(model.helpSearchMatches()) == 0 {
		test.Errorf("the search must still find matches")
	}
}

func TestHelpSearchNoMatchSays(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "help config")
	typeSearch(model, "zzzz")
	command := press(model, "enter")
	if command == nil {
		test.Fatal("expected a message")
	}
	if message, ok := command().(InfoMessage); !ok || message.Text != "no match: zzzz" {
		test.Errorf("message = %#v", command())
	}
}

func TestHelpSearchEscClearsThenCloses(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "help config")
	typeSearch(model, "wrapscan")
	press(model, "enter")
	press(model, "esc")
	if !model.showHelp || model.helpSearch.active() || markedRows(model) != 0 {
		test.Fatalf("first esc must clear the search and keep the overlay")
	}
	press(model, "esc")
	if model.showHelp {
		test.Errorf("second esc must close the overlay")
	}
}

func TestHelpSearchEscInThePromptGoesBack(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	typeSearch(model, "action 59")
	if model.helpOffset == 0 {
		test.Fatal("typing should have scrolled to the match")
	}
	press(model, "esc")
	if model.helpOffset != 0 || model.helpSearch.active() || !model.showHelp {
		test.Errorf("esc in the prompt must restore the view and the old search")
	}
}

func TestHelpSearchBelongsToTheOpenOverlay(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "help config")
	typeSearch(model, "wrapscan")
	press(model, "enter")
	press(model, "?")
	press(model, "?")
	if model.helpSearch.active() {
		test.Errorf("closing and reopening must start without a search")
	}
	typeSearch(model, "wrapscan")
	press(model, "enter")
	model.help("") // the command line is not reachable inside the overlay
	if model.helpSearch.active() {
		test.Errorf("switching page must clear the search")
	}
}

func TestHelpSearchSurvivesResize(test *testing.T) {
	model := sizedModel(100, 30)
	enterLine(model, "help config")
	typeSearch(model, "wrapscan")
	press(model, "enter")
	model.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	if !model.helpSearch.active() || len(model.helpSearchMatches()) == 0 || markedRows(model) == 0 {
		test.Errorf("the search must survive a resize and mark the re-laid-out rows")
	}
}

func TestHelpSearchGoldens(test *testing.T) {
	config := sizedModel(80, 20)
	enterLine(config, "help config")
	typeSearch(config, "resolved")
	press(config, "enter")
	golden.Assert(test, "help_search_config", screen(config), false)

	keys := sizedModel(80, 20)
	press(keys, "?")
	typeSearch(keys, "page")
	press(keys, "enter")
	golden.Assert(test, "help_search_keys", screen(keys), false)
}

func TestHelpSearchStepsFromTheScreenWhenTheMatchIsOffScreen(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	typeSearch(model, "action 5")
	press(model, "enter")
	press(model, "G")
	top := model.helpOffset
	if model.helpSearch.current >= top {
		test.Fatalf("setup: the current match %d should be above the screen top %d", model.helpSearch.current, top)
	}
	press(model, "n")
	if model.helpSearch.current < top {
		test.Errorf("n landed on %d, above the top of the screen %d", model.helpSearch.current, top)
	}
	pressAll(model, "g", "g", "G")
	top = model.helpOffset
	press(model, "N")
	if model.helpSearch.current >= top {
		test.Errorf("N landed on %d, want a match before the top of the screen %d", model.helpSearch.current, top)
	}
}

func TestHelpSearchEscKeepsTheLastQueryForRepeat(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "help config")
	typeSearch(model, "wrap")
	press(model, "enter")
	press(model, "esc")
	press(model, "/")
	press(model, "enter")
	if model.helpSearch.query != "wrap" {
		test.Errorf("query after repeat = %q, want wrap", model.helpSearch.query)
	}
}

func TestHelpSearchCancelRestoresTheEarlierSearch(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	typeSearch(model, "action")
	press(model, "enter")
	pressAll(model, "n", "n")
	current, offset := model.helpSearch.current, model.helpOffset
	typeSearch(model, "zzz")
	press(model, "esc")
	if model.helpSearch.query != "action" || model.helpSearch.current != current || model.helpOffset != offset {
		test.Fatalf("after esc: query %q current %d offset %d, want action, %d, %d",
			model.helpSearch.query, model.helpSearch.current, model.helpOffset, current, offset)
	}
	press(model, "n")
	matches := model.helpSearchMatches()
	want := matches[0]
	for i, match := range matches {
		if match == current && i+1 < len(matches) {
			want = matches[i+1]
		}
	}
	if model.helpSearch.current != want {
		test.Errorf("n went to %d, want %d (the match after %d)", model.helpSearch.current, want, current)
	}
}

func TestHelpSearchCountWithN(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	typeSearch(model, "action 5")
	press(model, "enter")
	matches := model.helpSearchMatches()
	pressAll(model, "3", "n")
	if model.helpSearch.current != matches[3] {
		test.Errorf("3n landed on %d, want the fourth match %d", model.helpSearch.current, matches[3])
	}
}

func TestHelpColonIsBlockedWhileSearching(test *testing.T) {
	model := sizedModel(80, 20)
	enterLine(model, "help config")
	typeSearch(model, "wrap")
	press(model, "enter")
	press(model, ":")
	if model.commandLine.active {
		test.Errorf(": opened the command line inside the help")
	}
}

func TestHelpOpeningCancelsTheViewsPrompt(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	press(model, "/")
	press(model, "a")
	follow(model, func() tea.Msg { return helpRequestedMessage{topic: "config"} })
	results := view.promptResults()
	if len(results) == 0 || results[len(results)-1].State != PromptCancelled {
		test.Fatalf("the view's prompt results = %#v, want a cancel last", results)
	}
	if !model.showHelp || model.commandLine.active {
		test.Fatalf("help open = %v, command line active = %v", model.showHelp, model.commandLine.active)
	}
	if last := results[len(results)-1]; last.Prefix != "/" || last.Text != "a" {
		test.Errorf("cancel = %#v, want prefix / and text a", last)
	}
	before := len(results)
	pressAll(model, "/", "w", "r")
	if len(view.promptResults()) != before {
		test.Errorf("keys typed in the help reached the view's search")
	}
	if model.helpSearch.query != "wr" {
		test.Errorf("help query = %q, want wr", model.helpSearch.query)
	}
}

func TestHelpOpeningCancelsTheHelpsOwnPrompt(test *testing.T) {
	view := newPrompting()
	model := newModel(view)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	press(model, ":")
	for _, character := range "help config" {
		press(model, string(character))
	}
	command := press(model, "enter") // the message has not landed yet
	pressAll(model, "?", "/", "a")
	follow(model, command)
	if !model.showHelp || model.helpContent != helpConfig || model.commandLine.active {
		test.Fatalf("help open = %v, content %v, command line active = %v", model.showHelp, model.helpContent, model.commandLine.active)
	}
	if results := view.promptResults(); len(results) != 0 {
		test.Errorf("the view heard %#v about a prompt it never opened", results)
	}
	if model.helpSearch.active() {
		test.Errorf("the cancelled search is still active: %q", model.helpSearch.query)
	}
}

func TestHelpSearchStepBoundaries(test *testing.T) {
	setup := func() (*Model, []int) {
		model := helpModel(newLongHelpView(60), 60, 12)
		typeSearch(model, "action 1")
		press(model, "enter")
		return model, model.helpSearchMatches()
	}
	_, height := sizedModel(60, 12).contentSize()
	rows := helpBodyRows(height)

	test.Run("N finds the match on the row just above the screen", func(test *testing.T) {
		model, matches := setup()
		model.helpOffset = matches[1] + 1
		model.clampHelp()
		if model.helpOffset != matches[1]+1 {
			test.Fatalf("setup: offset = %d, want %d", model.helpOffset, matches[1]+1)
		}
		model.helpSearch.current = matches[0]
		press(model, "N")
		if model.helpSearch.current != matches[1] {
			test.Errorf("N landed on %d, want the row just above the screen, %d", model.helpSearch.current, matches[1])
		}
	})

	test.Run("a match on the row just below the screen is off screen", func(test *testing.T) {
		model, matches := setup()
		model.helpOffset = matches[0]
		model.clampHelp()
		below := model.helpOffset + rows
		if !slices.Contains(matches, below) || model.helpOffset != matches[0] {
			test.Fatalf("setup: no match at row %d below the screen (matches %v)", below, matches)
		}
		model.helpSearch.current = below
		press(model, "n")
		if model.helpSearch.current != matches[0] {
			test.Errorf("n landed on %d, want the first match at the top of the screen, %d", model.helpSearch.current, matches[0])
		}
	})
}
