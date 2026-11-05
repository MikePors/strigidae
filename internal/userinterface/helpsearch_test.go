package userinterface

import (
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
