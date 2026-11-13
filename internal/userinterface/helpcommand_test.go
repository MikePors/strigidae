package userinterface

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/options"
)

// enterLine types a command line and runs the command it produces.
func enterLine(model *Model, line string) {
	press(model, ":")
	for _, character := range line {
		press(model, string(character))
	}
	follow(model, press(model, "enter"))
}

// follow runs a command and the commands its message produces, so a command
// that answers with an error to show has taken effect. Timers are instant, but status expiry is left unrun.
func follow(model *Model, command tea.Cmd) {
	if command == nil {
		return
	}
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, inner := range batch {
			follow(model, inner)
		}
		return
	}
	if _, expiry := message.(clearStatusMessage); expiry || message == nil {
		return // keep the status on screen for the test to read
	}
	_, next := model.Update(message)
	follow(model, next)
}

// screen is the content area of the model, without styling.
func screen(model *Model) string { return ansi.Strip(model.View().Content) }

func sizedModel(width, height int) *Model {
	model := newModel(newFake("v"))
	model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model
}

func TestHelpCommandOpensTheTopic(test *testing.T) {
	tests := []struct {
		name  string
		line  string
		title string
		want  []string
	}{
		{"no topic lists the topics", "help", "Help topics", []string{"keys", "config", "same as ?"}},
		{"keys", "help keys", "Help", []string{"Global", "toggle help"}},
		{"config", "help config", "Help: config", []string{"Option", "Value", "Default", "Description", "wrapscan", "relativenumber"}},
		{"spaces around the topic", "help   config ", "Help: config", []string{"wrapscan"}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := sizedModel(120, 40)
			enterLine(model, testCase.line)
			if !model.showHelp {
				test.Fatal("the overlay did not open")
			}
			text := screen(model)
			head := strings.Join(strings.Fields(strings.Split(text, "\n")[0]), " ")
			if !strings.HasPrefix(head, testCase.title+" ?") {
				test.Errorf("title line = %q, want it to start with %q", head, testCase.title)
			}
			for _, want := range testCase.want {
				if !strings.Contains(text, want) {
					test.Errorf("screen lacks %q:\n%s", want, text)
				}
			}
			press(model, "esc")
			if model.showHelp {
				test.Error("esc did not close the overlay")
			}
		})
	}
}

func TestHelpKeysIsTheQuestionMarkOverlay(test *testing.T) {
	question := sizedModel(100, 40)
	press(question, "?")
	command := sizedModel(100, 40)
	enterLine(command, "help keys")
	if diff := cmp.Diff(screen(question), screen(command)); diff != "" {
		test.Errorf(":help keys differs from ? (-?, +:help keys):\n%s", diff)
	}
}

func TestHelpUnknownTopicIsAnErrorAndOpensNothing(test *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"help nonsense", "unknown help topic or command: nonsense"},
		{"help keys extra", "unknown help topic or command: keys extra"},
	}
	for _, testCase := range tests {
		test.Run(testCase.line, func(test *testing.T) {
			model := sizedModel(100, 40)
			enterLine(model, testCase.line)
			if model.showHelp {
				test.Error("an unknown topic opened the overlay")
			}
			if model.status.kind != statusError || model.status.text != testCase.want {
				test.Errorf("status = %q (kind %v), want error %q", model.status.text, model.status.kind, testCase.want)
			}
		})
	}
}

// The command line strips escapes before the command runs, so the topic is
// given straight to the handler to prove the handler cleans it too.
func TestHelpUnknownTopicGivenDirectlyIsCleaned(test *testing.T) {
	model := sizedModel(100, 40)
	follow(model, model.help("foo\x1b[31m"))
	if model.showHelp {
		test.Error("an unknown topic opened the overlay")
	}
	if model.status.kind != statusError || strings.ContainsRune(model.status.text, '\x1b') || !strings.Contains(model.status.text, "foo") {
		test.Errorf("status = %q (kind %v), want an error naming foo without an escape byte", model.status.text, model.status.kind)
	}
}

func TestQuestionMarkAfterHelpConfigShowsTheKeyHelp(test *testing.T) {
	tests := []struct {
		name   string
		height int
		scroll bool
	}{
		{"at the top", 40, false},
		{"after scrolling the config list", 8, true},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			fresh := sizedModel(100, testCase.height)
			press(fresh, "?")

			model := sizedModel(100, testCase.height)
			enterLine(model, "help config")
			if testCase.scroll {
				if len(model.helpLines()) <= helpBodyRows(testCase.height-1) {
					test.Fatal("the config list fits: nothing to scroll")
				}
				pressAll(model, "G")
				if model.helpOffset == 0 {
					test.Fatal("the config list did not scroll")
				}
			}
			press(model, "esc")
			press(model, "?")
			if diff := cmp.Diff(screen(fresh), screen(model)); diff != "" {
				test.Errorf("? after :help config differs from ? on a fresh model (-fresh, +after):\n%s", diff)
			}
		})
	}
}

func TestHelpConfigShowsTheValueSetThisSession(test *testing.T) {
	model := sizedModel(120, 40)
	enterLine(model, "set wrap nowrapscan")
	enterLine(model, "help config")
	rows := map[string][]string{}
	for _, line := range strings.Split(screen(model), "\n") {
		if fields := strings.Fields(line); len(fields) >= 3 {
			rows[fields[0]] = fields[1:3]
		}
	}
	for name, want := range map[string][]string{
		"wrap":       {"wrap*", "nowrap"},
		"wrapscan":   {"nowrapscan*", "wrapscan"},
		"hlsearch":   {"hlsearch", "hlsearch"},
		"hideviewed": {"nohideviewed", "nohideviewed"},
	} {
		if diff := cmp.Diff(want, rows[name]); diff != "" {
			test.Errorf("row %s value and default (-want +got):\n%s", name, diff)
		}
	}
}

func TestHelpConfigListsEveryOption(test *testing.T) {
	model := sizedModel(160, 40)
	enterLine(model, "help config")
	text := screen(model)
	for _, option := range options.All() {
		if !strings.Contains(text, option.Name) || !strings.Contains(text, option.Description) {
			test.Errorf("option %q or its description is missing", option.Name)
		}
	}
}

func TestHelpEveryTopicIsListedAndCompletable(test *testing.T) {
	listing := sizedModel(120, 40)
	enterLine(listing, "help")
	text := screen(listing)

	completing := sizedModel(120, 40)
	_, candidates, ok := completing.completionCandidates("help ")
	if !ok {
		test.Fatal("nothing completes after help ")
	}
	for _, topic := range helpTopicTable() {
		if !strings.Contains(text, topic.name) || !strings.Contains(text, topic.description) {
			test.Errorf("topic %q is not listed by :help", topic.name)
		}
		if !slices.Contains(candidates, topic.name) {
			test.Errorf("topic %q is not completed after help ", topic.name)
		}
	}
	if len(candidates) != len(helpTopicTable())+len(CommandNames()) {
		test.Errorf("completion offers %v, want exactly the topics", candidates)
	}
}

func TestHelpTabCompletesTheTopic(test *testing.T) {
	model := newCompletionModel(newFake("v"))
	typeKeys(model, "help ", "tab")
	if got, want := model.commandLine.input.Value(), "help "+helpTopicTable()[0].name; got != want {
		test.Errorf("line = %q, want %q", got, want)
	}
	if got := len(shown(model)); got != len(helpTopicTable())+len(CommandNames()) {
		test.Errorf("%d matches shown, want %d", got, len(helpTopicTable())+len(CommandNames()))
	}
	model = newCompletionModel(newFake("v"))
	typeKeys(model, "help co", "tab")
	if got := model.commandLine.input.Value(); got != "help config" {
		test.Errorf("line = %q, want help config", got)
	}
}

func TestHelpCommandGoldens(test *testing.T) {
	topics := sizedModel(80, 12)
	enterLine(topics, "help")
	golden.Assert(test, "help_topics", screen(topics), false)

	config := sizedModel(100, 20)
	enterLine(config, "set wrap")
	enterLine(config, "help config")
	golden.Assert(test, "help_config", screen(config), false)

	scrolling := sizedModel(100, 8)
	enterLine(scrolling, "help config")
	if len(scrolling.helpLines()) <= helpBodyRows(7) {
		test.Fatal("the config list fits: nothing to scroll")
	}
	pressAll(scrolling, "G")
	golden.Assert(test, "help_config_scrolled", screen(scrolling), false)

	for _, width := range []int{80, 60} {
		narrow := sizedModel(width, 30)
		enterLine(narrow, "help config")
		golden.Assert(test, fmt.Sprintf("help_config_%d", width), screen(narrow), false)
	}

	narrowScrolling := sizedModel(60, 10)
	enterLine(narrowScrolling, "help config")
	pressAll(narrowScrolling, "G")
	golden.Assert(test, "help_config_narrow_scrolled", screen(narrowScrolling), false)
}

func TestWrapWords(test *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"fits exactly", "one two", 7, []string{"one two"}},
		{"breaks at a space", "one two three", 7, []string{"one two", "three"}},
		{"over-long word is hard-broken", "a abcdefghij b", 4, []string{"a", "abcd", "efgh", "ij b"}},
		{"empty", "", 5, []string{""}},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			if diff := cmp.Diff(testCase.want, wrapWords(testCase.text, testCase.width)); diff != "" {
				test.Errorf("wrapWords (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHelpConfigRelaysOutOnResize(test *testing.T) {
	model := sizedModel(120, 30)
	enterLine(model, "help config")
	wide := len(model.helpLines())
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	if narrow := len(model.helpLines()); narrow <= wide {
		test.Errorf("lines at 60 columns = %d, want more than %d at 120", narrow, wide)
	}
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if again := len(model.helpLines()); again != wide {
		test.Errorf("lines back at 120 columns = %d, want %d", again, wide)
	}
}

func TestHelpConfigMarksOnlyOptionsChangedFromTheDefault(test *testing.T) {
	model := sizedModel(120, 40)
	enterLine(model, "set wrap")
	enterLine(model, "help config")
	for _, line := range strings.Split(screen(model), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !slices.Contains(optionNames(), fields[0]) {
			continue
		}
		wantMarked := fields[0] == "wrap"
		if marked := strings.HasSuffix(fields[1], "*"); marked != wantMarked {
			test.Errorf("row %s: marked is %t, want %t (%q)", fields[0], marked, wantMarked, line)
		}
	}
	if !strings.Contains(screen(model), "* changed from the default") {
		test.Error("the legend is missing")
	}
}

func optionNames() []string {
	var names []string
	for _, option := range options.All() {
		names = append(names, option.Name)
	}
	return names
}

// The description column wraps in place at 20 columns and stacks at 19.
func TestHelpConfigLayoutThreshold(test *testing.T) {
	activeTheme := newModel(newFake("v")).theme
	var values options.Values
	// Far wider than needed, the heading shows where the description column starts.
	wide := ansi.Strip(helpConfigLines(activeTheme, values, 200)[0])
	leftWidth := strings.Index(wide, "Description")
	tests := []struct {
		name         string
		descriptions int
		wantStacked  bool
	}{
		{"19 stacks", 19, true},
		{"20 wraps in the column", 20, false},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			lines := helpConfigLines(activeTheme, values, leftWidth+testCase.descriptions)
			header := ansi.Strip(lines[0])
			if stacked := !strings.Contains(header, "Description"); stacked != testCase.wantStacked {
				test.Errorf("header %q: stacked = %v, want %v", header, stacked, testCase.wantStacked)
			}
			if testCase.wantStacked && !strings.HasPrefix(ansi.Strip(lines[2]), "    ") {
				test.Errorf("stacked description row = %q, want a 4-space indent", ansi.Strip(lines[2]))
			}
		})
	}
}

// Stacked descriptions wrap at the width minus their indent, so none overflows.
func TestHelpConfigStackedDescriptionsWrap(test *testing.T) {
	const width = 48
	activeTheme := newModel(newFake("v")).theme
	var descriptionRows int
	for _, line := range helpConfigLines(activeTheme, options.Values{}, width) {
		plain := ansi.Strip(line)
		if !strings.HasPrefix(plain, "    ") {
			continue // option rows are cut at the edge when narrower than their columns
		}
		descriptionRows++
		if got := lipgloss.Width(plain); got > width {
			test.Errorf("row %q is %d columns wide, want at most %d", plain, got, width)
		}
	}
	if optionCount := len(options.All()); descriptionRows <= optionCount {
		test.Errorf("%d stacked description rows for %d options: none wrapped", descriptionRows, optionCount)
	}
}

// Scrolled to the bottom, the offset follows the body through a resize narrower
// and back: it is clamped, the counter matches it and no blank tail shows.
func TestHelpConfigResizeKeepsTheScrollClamped(test *testing.T) {
	const height = 10
	model := sizedModel(120, height)
	enterLine(model, "help config")
	pressAll(model, "G")
	wantOffset := model.helpOffset
	for _, width := range []int{120, 60, 120} {
		model.Update(tea.WindowSizeMsg{Width: width, Height: height})
		lines := model.helpLines()
		_, contentHeight := model.contentSize()
		rows := helpBodyRows(contentHeight)
		wantOffset = min(wantOffset, len(lines)-rows)
		if model.helpOffset != wantOffset {
			test.Errorf("width %d: offset = %d, want %d", width, model.helpOffset, wantOffset)
		}
		screenLines := strings.Split(screen(model), "\n")
		counter := fmt.Sprintf("%d–%d of %d", wantOffset+1, wantOffset+rows, len(lines))
		if !strings.Contains(screenLines[0], counter) {
			test.Errorf("width %d: title %q lacks %q", width, screenLines[0], counter)
		}
		for index := range rows {
			want := strings.TrimRight(ansi.Strip(model.markedHelpLines()[wantOffset+index]), " ")
			if got := strings.TrimRight(screenLines[index+2], " "); got != want {
				test.Errorf("width %d: body row %d = %q, want %q", width, index, got, want)
			}
		}
	}
}
