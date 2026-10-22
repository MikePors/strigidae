package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// fakeView records what the root sends it.
type fakeView struct {
	title   string
	next    key.Binding // view-specific sequence "z q" (arbitrary; no real binding uses it)
	capture bool
	loading bool
	resumed int
	results int
	got     []tea.Msg
}

func newFake(title string) *fakeView {
	return &fakeView{
		title: title,
		next:  key.NewBinding(key.WithKeys("z q"), key.WithHelp("zq", "test sequence")),
	}
}

func (f *fakeView) Init() tea.Cmd { return nil }
func (f *fakeView) Loading() bool { return f.loading }
func (f *fakeView) Resumed() tea.Cmd {
	f.resumed++
	return nil
}

func (f *fakeView) Update(msg tea.Msg) (View, tea.Cmd) {
	f.got = append(f.got, msg)
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		f.capture = false
	}
	if _, ok := msg.(resultMsg); ok {
		f.results++
		f.loading = false
	}
	return f, nil
}
func (f *fakeView) Render(_, _ int) string    { return f.title + " body" }
func (f *fakeView) Title() string             { return f.title }
func (f *fakeView) ShortHelp() []key.Binding  { return []key.Binding{f.next} }
func (f *fakeView) FullHelp() [][]key.Binding { return [][]key.Binding{{f.next}} }
func (f *fakeView) CapturesKeys() bool        { return f.capture }
func (f *fakeView) last() tea.Msg             { return f.got[len(f.got)-1] }
func (f *fakeView) keyMsgs() (out []keys.Msg) {
	for _, m := range f.got {
		if k, ok := m.(keys.Msg); ok {
			out = append(out, k)
		}
	}
	return out
}

// instantTick makes timers return their message immediately when the command
// is run, so tests need no clock or sleeping.
func instantTick(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg { return fn(time.Time{}) }
}

func newModel(root View) *Model {
	m := New(root, theme.Default(), Options{Tick: instantTick})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	return m
}

func press(m *Model, s string) tea.Cmd {
	_, cmd := m.Update(keyFor(s))
	return cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func run(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if _, ok := msg.(tea.BatchMsg); ok {
		return
	}
	m.Update(msg)
}

func TestKeyEncoding(t *testing.T) {
	// Guards the assumption press() relies on: key names match binding names.
	for _, s := range []string{"j", "G", "?", ":", "ctrl+c", "esc", "enter", "tab", "ctrl+o"} {
		k := keyFor(s)
		if k.String() != s {
			t.Errorf("key for %q stringifies as %q", s, k.String())
		}
	}
}

func keyFor(s string) tea.KeyPressMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func TestCommandLine(t *testing.T) {
	typeLine := func(m *Model, line string) tea.Cmd {
		press(m, ":")
		for _, r := range line {
			press(m, string(r))
		}
		return press(m, "enter")
	}
	t.Run(": opens and captures keys", func(t *testing.T) {
		v := newFake("v")
		m := newModel(v)
		press(m, ":")
		if !m.cmdline.active {
			t.Fatal(": should open the command line")
		}
		press(m, "j")
		if len(v.keyMsgs()) != 0 || m.cmdline.input.Value() != "j" {
			t.Errorf("keys leaked to view or were not captured; value = %q", m.cmdline.input.Value())
		}
	})
	t.Run(":q on root quits", func(t *testing.T) {
		m := newModel(newFake("root"))
		_, cmd := m.Update(typeLine(m, "q")())
		if !isQuit(cmd) {
			t.Error(":q on the root view should quit")
		}
	})
	t.Run(":q pops a pushed view", func(t *testing.T) {
		m := newModel(newFake("root"))
		run(m, Push(newFake("child")))
		run(m, typeLine(m, "quit"))
		if len(m.stack) != 1 {
			t.Errorf("depth = %d, want 1", len(m.stack))
		}
	})
	t.Run(":qa quits from anywhere", func(t *testing.T) {
		m := newModel(newFake("root"))
		run(m, Push(newFake("child")))
		cmd := typeLine(m, "qa")
		if cmd == nil {
			t.Fatal("expected a command")
		}
		if _, ok := cmd().(QuitMsg); !ok {
			t.Error(":qa should produce QuitMsg")
		}
	})
	t.Run("unknown command shows an error", func(t *testing.T) {
		m := newModel(newFake("root"))
		run(m, typeLine(m, "frobnicate now"))
		if m.status.text != "not a command: frobnicate" || m.status.kind != statusError {
			t.Errorf("status = %+v", m.status)
		}
	})
	t.Run("esc cancels", func(t *testing.T) {
		m := newModel(newFake("root"))
		press(m, ":")
		press(m, "q")
		press(m, "esc")
		if m.cmdline.active || len(m.stack) != 1 {
			t.Error("esc should cancel the command line without running it")
		}
	})
	t.Run("backspace on empty line cancels, otherwise deletes", func(t *testing.T) {
		m := newModel(newFake("root"))
		press(m, ":")
		press(m, "q")
		m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		if !m.cmdline.active || m.cmdline.input.Value() != "" {
			t.Fatal("first backspace should delete the character")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		if m.cmdline.active {
			t.Error("backspace on an empty line should cancel")
		}
	})
	t.Run("ctrl-c and q do nothing special", func(t *testing.T) {
		m := newModel(newFake("root"))
		if cmd := press(m, "ctrl+c"); isQuit(cmd) {
			t.Error("ctrl+c must not quit")
		}
		if cmd := press(m, "q"); isQuit(cmd) || len(m.stack) != 1 {
			t.Error("bare q must not quit or pop")
		}
	})
	t.Run("tab and ctrl+o are not intercepted", func(t *testing.T) {
		m := newModel(newFake("root"))
		press(m, "tab")
		press(m, "ctrl+o")
		if m.cmdline.active || m.showHelp || len(m.stack) != 1 {
			t.Error("reserved keys must not navigate")
		}
	})
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		line string
		want tea.Msg // nil: expect a "not a command" error for wantName
		name string
	}{
		{line: "q", want: PopMsg{}},
		{line: "quit", want: PopMsg{}},
		{line: "  q  ", want: PopMsg{}},
		{line: "qa", want: QuitMsg{}},
		{line: "qall", want: QuitMsg{}},
		{line: "quitall", want: QuitMsg{}},
		{line: "zzz arg", name: "zzz"},
		{line: "Q", name: "Q"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parseCommand(tt.line)()
			if tt.want == nil {
				em, ok := got.(ErrorMsg)
				if !ok || em.Err.Error() != "not a command: "+tt.name {
					t.Errorf("got %#v, want not-a-command error for %q", got, tt.name)
				}
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
	if parseCommand("   ") != nil {
		t.Error("blank line should do nothing")
	}
}

// TestCapturersReleaseOnEsc is the safety net for having no ctrl+c: every
// component that captures keys must release capture on esc.
func TestCapturersReleaseOnEsc(t *testing.T) {
	tests := []struct {
		name string
		// setup returns a root with the capturer engaged, and a func reporting
		// whether it still captures.
		setup func() (*Model, func() bool)
	}{
		{"command line", func() (*Model, func() bool) {
			m := newModel(newFake("v"))
			press(m, ":")
			press(m, "x")
			return m, m.cmdline.CapturesKeys
		}},
		{"view implementing KeyCapturer", func() (*Model, func() bool) {
			v := newFake("v")
			v.capture = true
			return newModel(v), v.CapturesKeys
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, captures := tt.setup()
			if !captures() {
				t.Fatal("setup did not engage capture")
			}
			press(m, "esc")
			if captures() {
				t.Error("capturer still holds keys after esc")
			}
		})
	}
}

func TestPushPop(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	m := newModel(root)

	run(m, Push(child))
	if m.top() != View(child) || len(m.stack) != 2 {
		t.Fatalf("push failed: depth %d", len(m.stack))
	}
	if diff := cmp.Diff(tea.WindowSizeMsg{Width: 60, Height: 11}, child.got[0]); diff != "" {
		t.Errorf("pushed view should get content size (-want +got):\n%s", diff)
	}
	if got := m.bar().crumbs; got != "root › child" {
		t.Errorf("crumbs = %q", got)
	}

	run(m, Pop())
	if m.top() != View(root) || len(m.stack) != 1 {
		t.Errorf("pop failed: depth %d", len(m.stack))
	}

	run(m, Push(nil))
	if len(m.stack) != 1 {
		t.Error("pushing nil should be ignored")
	}
	_, cmd := m.Update(PopMsg{})
	if !isQuit(cmd) {
		t.Error("PopMsg on root should quit")
	}
}

func TestSequences(t *testing.T) {
	t.Run("global gg delivered once complete", func(t *testing.T) {
		v := newFake("v")
		m := newModel(v)
		if cmd := press(m, "g"); cmd == nil {
			t.Fatal("first g should schedule a timeout")
		}
		if m.matcher.Display() != "g" || m.bar().pending != "g" {
			t.Errorf("pending prefix = %q, want g", m.bar().pending)
		}
		if len(v.keyMsgs()) != 0 {
			t.Error("view received keys before sequence completed")
		}
		press(m, "g")
		if diff := cmp.Diff([]keys.Msg{{Seq: "g g"}}, v.keyMsgs()); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if m.bar().pending != "" {
			t.Error("pending prefix should clear after match")
		}
	})
	t.Run("view-declared sequence", func(t *testing.T) {
		v := newFake("v")
		m := newModel(v)
		press(m, "z")
		press(m, "q")
		if diff := cmp.Diff([]keys.Msg{{Seq: "z q"}}, v.keyMsgs()); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
	t.Run("timeout clears pending prefix", func(t *testing.T) {
		v := newFake("v")
		m := newModel(v)
		cmd := press(m, "g")
		run(m, cmd) // fires seqTimeoutMsg
		if m.matcher.Pending() {
			t.Error("timeout should clear the pending prefix")
		}
		if len(v.keyMsgs()) != 0 {
			t.Error("incomplete sequence must not reach the view")
		}
	})
	t.Run("stale timeout is ignored", func(t *testing.T) {
		m := newModel(newFake("v"))
		stale := press(m, "g")
		press(m, "g")
		press(m, "z") // new pending prefix
		run(m, stale)
		if !m.matcher.Pending() {
			t.Error("stale timeout cleared a newer prefix")
		}
	})
	t.Run("unbound key goes to view raw", func(t *testing.T) {
		v := newFake("v")
		m := newModel(v)
		press(m, "x")
		if _, ok := v.last().(tea.KeyPressMsg); !ok {
			t.Errorf("last msg = %T, want tea.KeyPressMsg", v.last())
		}
	})
}

func TestKeyCapture(t *testing.T) {
	v := newFake("v")
	v.capture = true
	m := newModel(v)
	press(m, ":")
	press(m, "?")
	if m.cmdline.active || m.showHelp {
		t.Error("captured keys must not open the command line or help")
	}
	if len(v.got) != 3 { // size + : + ?
		t.Errorf("view got %d msgs, want 3", len(v.got))
	}
}

func TestHelpToggle(t *testing.T) {
	v := newFake("v")
	m := newModel(v)

	press(m, "?")
	if !m.showHelp {
		t.Fatal("? should open help")
	}
	press(m, "j")
	if len(v.keyMsgs()) != 0 {
		t.Error("keys must not reach the view while help is open")
	}
	for _, closer := range []string{"?", "esc"} {
		m.showHelp = true
		press(m, closer)
		if m.showHelp {
			t.Errorf("%s should close help", closer)
		}
	}
	if len(m.stack) != 1 {
		t.Error("closing help must not pop")
	}
}

func TestStatusMessages(t *testing.T) {
	m := newModel(newFake("v"))

	_, cmd := m.Update(InfoMsg{Text: "hello"})
	if m.status.text != "hello" || m.status.kind != statusInfo {
		t.Errorf("status = %+v", m.status)
	}
	stale := cmd()

	_, cmd = m.Update(ErrorMsg{Err: errors.New("boom")})
	if m.status.text != "boom" || m.status.kind != statusError {
		t.Errorf("status = %+v", m.status)
	}
	m.Update(stale) // the older info timer must not clear the newer error
	if m.status.text != "boom" {
		t.Error("stale clear removed a newer message")
	}
	m.Update(cmd())
	if m.status.text != "" || m.status.kind != statusNone {
		t.Errorf("message should auto-clear, got %+v", m.status)
	}
}

// resultMsg stands in for a command result addressed to a view.
type resultMsg struct{}

func TestSpinnerFollowsTopView(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	root.loading = true
	m := newModel(root)

	if !m.spinning || m.bar().spinner == "" {
		t.Fatal("a loading top view should start the spinner")
	}
	run(m, Push(child))
	if m.bar().spinner != "" {
		t.Error("spinner must follow the top view, which is not loading")
	}
	run(m, Pop())
	if m.bar().spinner == "" {
		t.Error("spinner should return when the loading view is revealed again")
	}
	root.loading = false
	m.Update(resultMsg{})
	if m.bar().spinner != "" {
		t.Error("spinner should stop when the view stops loading")
	}
}

func TestPoppedViewEndsItsSpinner(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	m := newModel(root)
	run(m, Push(child))
	child.loading = true
	m.Update(struct{}{}) // any message makes the root re-check
	if m.bar().spinner == "" {
		t.Fatal("child is loading; spinner expected")
	}
	run(m, Pop())
	if m.bar().spinner != "" {
		t.Error("popping the loading view must end its spinner")
	}
}

func TestResultReachesCoveredView(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	root.loading = true
	m := newModel(root)
	run(m, Push(child)) // pushed while root's load is in flight

	m.Update(resultMsg{}) // the load finishes while child is on top
	if root.results != 1 || child.results != 1 {
		t.Fatalf("result delivery: root %d, child %d; want both 1 (broadcast)", root.results, child.results)
	}
	run(m, Pop())
	if root.loading || m.bar().spinner != "" {
		t.Error("after :q the lower view should show its result with no stuck spinner")
	}
}

func TestResumed(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	m := newModel(root)
	run(m, Push(child))
	if root.resumed != 0 {
		t.Fatal("push must not resume")
	}
	run(m, Pop())
	if root.resumed != 1 || child.resumed != 0 {
		t.Errorf("resumed: root %d, child %d; want 1, 0", root.resumed, child.resumed)
	}
}

func TestSpinnerTicks(t *testing.T) {
	v := newFake("v")
	m := newModel(v)

	own := spinner.New().Tick() // a tick from a spinner the view owns
	before := len(v.got)
	m.Update(own)
	if len(v.got) != before+1 {
		t.Error("a view's own spinner tick must reach the view")
	}

	rootTick := m.spin.Tick()
	before = len(v.got)
	m.Update(rootTick)
	if len(v.got) != before {
		t.Error("the root spinner's tick must be consumed, not forwarded")
	}
}

func TestResizeAndTheme(t *testing.T) {
	root, child := newFake("root"), newFake("child")
	m := newModel(root)
	run(m, Push(child))

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	want := tea.WindowSizeMsg{Width: 100, Height: 29}
	for name, v := range map[string]*fakeView{"root": root, "child": child} {
		if diff := cmp.Diff(want, v.last()); diff != "" {
			t.Errorf("%s resize mismatch (-want +got):\n%s", name, diff)
		}
	}

	m.Update(tea.BackgroundColorMsg{})
	if _, ok := child.last().(ThemeMsg); !ok {
		t.Errorf("child last msg = %T, want ThemeMsg", child.last())
	}
}

func TestViewDimensions(t *testing.T) {
	m := newModel(newFake("v"))
	got := m.View()
	if !got.AltScreen {
		t.Error("view should use the alt screen")
	}
	golden.Assert(t, "model_view", got.Content, false)
}

func TestHelpGolden(t *testing.T) {
	m := newModel(newFake("v"))
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	press(m, "?")
	golden.Assert(t, "help_overlay", m.View().Content, false)
}

func TestStatusBarGolden(t *testing.T) {
	th := theme.Default()
	tests := []struct {
		name string
		bar  statusBar
		w    int
	}{
		{"idle", statusBar{crumbs: "strig › screen 2", text: "? help  enter open"}, 60},
		{"pending_spinner", statusBar{crumbs: "strig", text: "? help", pending: "g", spinner: "⣾"}, 60},
		{"error", statusBar{crumbs: "strig", text: "load failed: rate limited", kind: statusError}, 60},
		{"info_narrow", statusBar{crumbs: "strig", text: "a rather long informational message", kind: statusInfo, pending: "g"}, 30},
		{"tiny", statusBar{crumbs: "a very long breadcrumb trail"}, 10},
		{"long_crumbs_error_pending", statusBar{
			crumbs:  "owner/repo \u203a #123 Fix the thing \u203a internal/ui/model.go",
			text:    "post failed: 422",
			kind:    statusError,
			pending: "g",
			spinner: "\u28fe",
		}, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.bar.render(th, tt.w)
			if w := lipgloss.Width(got); w != tt.w {
				t.Errorf("width = %d, want %d", w, tt.w)
			}
			golden.Assert(t, "statusbar_"+tt.name, got, false)
		})
	}
}

func TestStatusBarStyled(t *testing.T) {
	th := theme.Default()
	got := statusBar{crumbs: "x", text: "oops", kind: statusError}.render(th, 20)
	if got == ansi.Strip(got) {
		t.Error("expected ANSI styling in the rendered bar")
	}
}

func TestCommandLineGolden(t *testing.T) {
	m := newModel(newFake("v"))
	press(m, ":")
	press(m, "q")
	press(m, "a")
	golden.Assert(t, "command_line", m.View().Content, false)
}

func TestCommandLineStaysOneRow(t *testing.T) {
	m := newModel(newFake("v"))
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 6})
	press(m, ":")
	for range 60 {
		press(m, "x")
	}
	got := m.View().Content
	if n := lipgloss.Height(got); n != 6 {
		t.Errorf("frame height = %d, want 6", n)
	}
	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("line %d is %d columns wide, want <= 20", i, w)
		}
	}
	golden.Assert(t, "command_line_overflow", got, false)
}
