package userinterface

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/golden"
)

// longHelpView lists many view bindings, so the help does not fit a short
// terminal. Disabling the bindings shortens the help while it is open.
type longHelpView struct {
	*fakeView
	bindings []key.Binding
}

func newLongHelpView(count int) *longHelpView {
	view := &longHelpView{fakeView: newFake("v")}
	for i := range count {
		name := fmt.Sprintf("F%d", i)
		view.bindings = append(view.bindings, key.NewBinding(key.WithKeys(name), key.WithHelp(name, fmt.Sprintf("action %d", i))))
	}
	return view
}

// Update keeps the stack entry as the wrapper; the embedded view would
// return itself and drop the extra bindings.
func (view *longHelpView) Update(message tea.Msg) (View, tea.Cmd) {
	_, command := view.fakeView.Update(message)
	return view, command
}

func (view *longHelpView) FullHelp() [][]key.Binding { return [][]key.Binding{view.bindings} }

func (view *longHelpView) setEnabled(enabled bool) {
	for i := range view.bindings {
		view.bindings[i].SetEnabled(enabled)
	}
}

// helpModel opens the help on a terminal of the given size.
func helpModel(view View, width, height int) *Model {
	model := newModel(view)
	model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	press(model, "?")
	return model
}

func pressAll(model *Model, texts ...string) {
	for _, text := range texts {
		press(model, text)
	}
}

// helpHead returns the title line of the help as shown.
func helpHead(model *Model) string {
	return strings.TrimRight(ansi.Strip(strings.Split(model.View().Content, "\n")[0]), " ")
}

// helpRange returns the indicator text after "? / esc to close".
func helpRange(model *Model) string {
	head := helpHead(model)
	index := strings.Index(head, "close")
	return strings.TrimSpace(head[index+len("close"):])
}

func TestHelpScrollKeys(test *testing.T) {
	// 60x12 gives 11 content rows and 9 body rows.
	total := len(helpModel(newLongHelpView(60), 60, 12).helpLines())
	last := fmt.Sprintf("%d–%d of %d", total-8, total, total)
	first := fmt.Sprintf("1–9 of %d", total)
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"top", nil, first},
		{"down", []string{"j"}, fmt.Sprintf("2–10 of %d", total)},
		{"down count", []string{"3", "j"}, fmt.Sprintf("4–12 of %d", total)},
		{"up at top", []string{"k"}, first},
		{"up", []string{"5", "j", "2", "k"}, fmt.Sprintf("4–12 of %d", total)},
		{"half down", []string{"ctrl+d"}, fmt.Sprintf("5–13 of %d", total)},
		{"half up", []string{"ctrl+d", "ctrl+d", "ctrl+u"}, fmt.Sprintf("5–13 of %d", total)},
		{"page down", []string{"ctrl+f"}, fmt.Sprintf("10–18 of %d", total)},
		{"page down count", []string{"2", "ctrl+f"}, fmt.Sprintf("19–27 of %d", total)},
		{"page up", []string{"ctrl+f", "ctrl+f", "ctrl+b"}, fmt.Sprintf("10–18 of %d", total)},
		{"bottom", []string{"G"}, last},
		{"bottom then down", []string{"G", "j", "j"}, last},
		{"bottom then up", []string{"G", "j", "j", "k"}, fmt.Sprintf("%d–%d of %d", total-9, total-1, total)},
		{"page down past end", []string{"2", "0", "ctrl+f"}, last},
		{"top after bottom", []string{"G", "g", "g"}, first},
		{"top ignores count", []string{"G", "4", "g", "g"}, first},
		{"line count", []string{"5", "G"}, fmt.Sprintf("5–13 of %d", total)},
		{"line count past end", []string{"9", "9", "9", "G"}, last},
		{"unrelated keys", []string{"o", "enter", "R", "n"}, first},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			model := helpModel(newLongHelpView(60), 60, 12)
			pressAll(model, testCase.keys...)
			if got := helpRange(model); got != testCase.want {
				test.Errorf("indicator = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestHelpKeysNeverReachTheView(test *testing.T) {
	view := newLongHelpView(60)
	model := helpModel(view, 60, 12)
	pressAll(model, "j", "g", "g", "G", "ctrl+d", "ctrl+f", "o", ":", "F1")
	if got := len(view.keyMessages()); got != 0 {
		test.Errorf("%d keys reached the view", got)
	}
	if model.commandLine.active {
		test.Error(": must not open the command line while the help is open")
	}
}

func TestHelpThatFitsIgnoresKeys(test *testing.T) {
	model := helpModel(newFake("v"), 100, 50)
	before := model.View().Content
	pressAll(model, "j", "G", "ctrl+f", "5", "j")
	if strings.Contains(helpHead(model), " of ") {
		test.Errorf("fitting help shows an indicator: %q", helpHead(model))
	}
	if after := model.View().Content; after != before {
		test.Error("keys changed a help that fits")
	}
}

func TestHelpClosesWithPendingKeys(test *testing.T) {
	tests := []struct {
		name    string
		pending []string
		closer  string
	}{
		{"g then question mark", []string{"g"}, "?"},
		{"g then escape", []string{"g"}, "esc"},
		{"count then question mark", []string{"1", "2"}, "?"},
		{"count then escape", []string{"1", "2"}, "esc"},
	}
	for _, testCase := range tests {
		test.Run(testCase.name, func(test *testing.T) {
			view := newLongHelpView(60)
			model := helpModel(view, 60, 12)
			press(model, "G")
			pressAll(model, testCase.pending...)
			press(model, testCase.closer)
			if model.showHelp {
				test.Fatal("help still open")
			}
			if model.matcher.Pending() || model.bar().pending != "" {
				test.Error("pending keys survived closing the help")
			}
			// The next key is a fresh one for the view, not the end of g g or 12j.
			press(model, "j")
			messages := view.keyMessages()
			if len(messages) != 1 || messages[0].Sequence != "j" || messages[0].Count != 0 {
				test.Errorf("view got %+v, want one plain j", messages)
			}
			press(model, "?")
			if !strings.Contains(helpHead(model), "1–9 of") {
				test.Errorf("reopened help not at the top: %q", helpHead(model))
			}
		})
	}
}

func TestHelpPendingTimeoutStaysInTheHelp(test *testing.T) {
	// "g" is a complete binding of the view and a prefix of the global "g g".
	// The help matches only the global bindings, so its timeout must neither
	// find the view's binding nor send anything to the view.
	view := newLongHelpView(60)
	view.bindings = append(view.bindings, key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "view goto")))
	model := helpModel(view, 60, 12)
	press(model, "G")
	scrolled := helpRange(model)
	command := press(model, "g")
	if command == nil || !model.matcher.Pending() {
		test.Fatal("g should be pending in the help")
	}
	run(model, command) // the timeout fires
	if got := len(view.keyMessages()); got != 0 {
		test.Errorf("timeout sent %d keys to the view", got)
	}
	if model.matcher.Pending() {
		test.Error("the timeout left a pending key")
	}
	if got := helpRange(model); got != scrolled {
		test.Errorf("indicator = %q, want it unchanged at %q", got, scrolled)
	}
}

func TestHelpGrowsAndShrinksWithTheTerminal(test *testing.T) {
	model := helpModel(newLongHelpView(60), 60, 12)
	press(model, "G")
	total := len(model.helpLines())

	model.Update(tea.WindowSizeMsg{Width: 60, Height: 22}) // 19 body rows
	if got, want := helpRange(model), fmt.Sprintf("%d–%d of %d", total-18, total, total); got != want {
		test.Errorf("after growing: %q, want %q", got, want)
	}
	pressAll(model, "k", "k")
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	if got, want := helpRange(model), fmt.Sprintf("%d–%d of %d", total-20, total-12, total); got != want {
		test.Errorf("after shrinking: %q, want the top line kept: %q", got, want)
	}
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 100}) // fits now
	if strings.Contains(helpHead(model), " of ") {
		test.Errorf("indicator on a help that fits: %q", helpHead(model))
	}
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	if got := helpRange(model); !strings.HasSuffix(got, fmt.Sprintf(" of %d", total)) {
		test.Errorf("after fitting then shrinking: %q", got)
	}
}

func TestHelpBodyShrinksWhileOpen(test *testing.T) {
	view := newLongHelpView(60)
	model := helpModel(view, 60, 12)
	press(model, "G")
	view.setEnabled(false)
	short := len(model.helpLines())
	if short <= 9 {
		test.Fatalf("test needs a body still longer than the window, got %d", short)
	}
	press(model, "j") // at the bottom already: must not bank an offset
	if got, want := helpRange(model), fmt.Sprintf("%d–%d of %d", short-8, short, short); got != want {
		test.Errorf("after the body shrank: %q, want %q", got, want)
	}
	press(model, "k")
	if got, want := helpRange(model), fmt.Sprintf("%d–%d of %d", short-9, short-1, short); got != want {
		test.Errorf("k from the bottom: %q, want %q", got, want)
	}
}

func TestHelpRenderClampsWithoutAKey(test *testing.T) {
	// A load can shorten the body with no key or resize to clamp the offset.
	view := newLongHelpView(60)
	model := helpModel(view, 60, 12)
	press(model, "G")
	view.setEnabled(false)
	short := len(model.helpLines())
	if got, want := helpRange(model), fmt.Sprintf("%d–%d of %d", short-8, short, short); got != want {
		test.Errorf("indicator = %q, want %q", got, want)
	}
}

func TestHelpNeverShowsAnEmptyBody(test *testing.T) {
	for _, height := range []int{2, 3, 4, 5} {
		test.Run(fmt.Sprintf("height %d", height), func(test *testing.T) {
			model := helpModel(newLongHelpView(60), 60, height)
			pressAll(model, "G", "j")
			lines := strings.Split(ansi.Strip(model.View().Content), "\n")
			if height >= 4 && strings.TrimSpace(lines[2]) == "" {
				test.Errorf("empty body at height %d: %q", height, lines)
			}
		})
	}
}

func TestHelpScrollGolden(test *testing.T) {
	model := helpModel(newLongHelpView(12), 70, 14)
	if total := len(model.helpLines()); total <= helpBodyRows(13) {
		test.Fatalf("golden help fits (%d lines)", total)
	}
	for _, step := range []struct {
		name string
		keys []string
	}{
		{"top", nil},
		{"middle", []string{"1", "0", "j"}},
		{"bottom", []string{"G"}},
	} {
		pressAll(model, step.keys...)
		golden.Assert(test, "help_scroll_"+step.name, ansi.Strip(model.View().Content), false)
	}
}
