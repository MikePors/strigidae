package ui

import (
	"errors"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
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
	next    key.Binding // view-specific sequence "] f"
	capture bool
	got     []tea.Msg
}

func newFake(title string) *fakeView {
	return &fakeView{
		title: title,
		next:  key.NewBinding(key.WithKeys("] f"), key.WithHelp("]f", "next file")),
	}
}

func (f *fakeView) Init() tea.Cmd { return nil }
func (f *fakeView) Update(msg tea.Msg) (View, tea.Cmd) {
	f.got = append(f.got, msg)
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
	for _, s := range []string{"j", "G", "?", "ctrl+c", "esc", "enter"} {
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
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func TestQuitAndBack(t *testing.T) {
	t.Run("q on root quits", func(t *testing.T) {
		m := newModel(newFake("root"))
		if !isQuit(press(m, "q")) {
			t.Error("q on root view should quit")
		}
	})
	t.Run("ctrl-c quits from anywhere", func(t *testing.T) {
		m := newModel(newFake("root"))
		run(m, Push(newFake("child")))
		press(m, "?")
		if !isQuit(press(m, "ctrl+c")) {
			t.Error("ctrl+c should quit even with help open on a pushed view")
		}
	})
	t.Run("QuitMsg quits", func(t *testing.T) {
		m := newModel(newFake("root"))
		_, cmd := m.Update(QuitMsg{})
		if !isQuit(cmd) {
			t.Error("QuitMsg should quit")
		}
	})
	t.Run("esc is passed to the view, not used for popping", func(t *testing.T) {
		root, child := newFake("root"), newFake("child")
		m := newModel(root)
		run(m, Push(child))
		press(m, "esc")
		if len(m.stack) != 2 {
			t.Errorf("stack depth = %d, want 2", len(m.stack))
		}
		if diff := cmp.Diff([]keys.Msg{{Seq: "esc"}}, child.keyMsgs()); diff != "" {
			t.Errorf("child key msgs mismatch (-want +got):\n%s", diff)
		}
	})
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

	if cmd := press(m, "q"); cmd != nil {
		t.Errorf("q on child returned a command, want nil")
	}
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
		press(m, "]")
		press(m, "f")
		if diff := cmp.Diff([]keys.Msg{{Seq: "] f"}}, v.keyMsgs()); diff != "" {
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
		press(m, "]") // new pending prefix
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
	press(m, "q")
	press(m, "?")
	if len(m.stack) != 1 || m.showHelp {
		t.Error("captured keys must not trigger back or help")
	}
	if len(v.got) != 3 { // size + q + ?
		t.Errorf("view got %d msgs, want 3", len(v.got))
	}
	if !isQuit(press(m, "ctrl+c")) {
		t.Error("ctrl+c must still quit while capturing")
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
	for _, closer := range []string{"?", "q", "esc"} {
		m.showHelp = true
		press(m, closer)
		if m.showHelp {
			t.Errorf("%s should close help", closer)
		}
	}
	if len(m.stack) != 1 {
		t.Error("closing help with q must not pop")
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

func TestLoading(t *testing.T) {
	m := newModel(newFake("v"))

	_, cmd := m.Update(LoadingMsg{Key: "a", Loading: true})
	if cmd == nil || m.bar().spinner == "" {
		t.Error("loading should start the spinner")
	}
	_, cmd = m.Update(LoadingMsg{Key: "b", Loading: true})
	if cmd != nil {
		t.Error("second load must not start a second tick chain")
	}
	m.Update(LoadingMsg{Key: "a", Loading: false})
	if m.bar().spinner == "" {
		t.Error("spinner should stay while another load is active")
	}
	m.Update(LoadingMsg{Key: "b", Loading: false})
	if m.bar().spinner != "" {
		t.Error("spinner should stop when all loads finish")
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
		{"info_narrow", statusBar{crumbs: "strig", text: "a rather long informational message", kind: statusInfo, pending: "]"}, 30},
		{"tiny", statusBar{crumbs: "a very long breadcrumb trail"}, 10},
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
