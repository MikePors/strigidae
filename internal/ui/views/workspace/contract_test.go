package workspace

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

func TestDiffRefreshFailureKeepsDiff(t *testing.T) {
	h := loaded(t, 100, 30)
	before := h.v.diff.Raw
	h.client.SetError(fake.MethodGetDiff, errors.New("blip"))
	h.press("R")
	if h.v.diff.Raw != before || h.v.diff.Loading {
		t.Errorf("diff after failed refresh = %+v, want Raw kept and not loading", h.v.diff)
	}
	if h.lastErr() == nil {
		t.Error("diff failure should reach the status bar")
	}
	// A too-large diff clears the text.
	h.client.SetError(fake.MethodGetDiff, github.ErrDiffTooLarge)
	h.press("R")
	if h.v.diff.Raw != "" || !errors.Is(h.v.diff.Err, github.ErrDiffTooLarge) {
		t.Errorf("too-large diff = %+v", h.v.diff)
	}
}

func TestPickerChoosesByPathAcrossRefresh(t *testing.T) {
	h := loaded(t, 100, 24)
	h.press("f")
	h.typ("legacy")
	// A refresh lands while the picker is open, with a new file sorted first.
	d := testDetail()
	d.Files = append([]github.ChangedFile{{Path: "aaa/new.go", Additions: 1, Change: github.FileAdded}}, d.Files...)
	h.send(prLoadedMsg{repo: testRepo, number: 7, seq: h.v.seq, detail: d})
	h.send(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := h.v.selectedPath(); got != "internal/sync/legacy.go" {
		t.Errorf("picker selected %q, want legacy.go", got)
	}
}

func TestRelativeTimesAge(t *testing.T) {
	now := testNow
	h := newHarness(t, 100, 60)
	h.v.now = func() time.Time { return now }
	h.run(h.v.Init())
	h.press("l", "G")
	if !strings.Contains(ansi.Strip(h.render()), "bob approved · 1h ago") {
		t.Fatal("setup: expected 1h ago")
	}
	now = testNow.Add(2 * time.Hour)
	if out := ansi.Strip(h.render()); !strings.Contains(out, "bob approved · 3h ago") {
		t.Errorf("relative time did not age:\n%s", out)
	}
}

// stubPane records what the workspace tells a FilePane.
type stubPane struct {
	files []string
	sizes [][2]int
	keys  []string
}

func (p *stubPane) SetTheme(theme.Theme)         {}
func (p *stubPane) SetDiff(DiffState)            {}
func (p *stubPane) SetFile(f github.ChangedFile) { p.files = append(p.files, f.Path) }
func (p *stubPane) SetSize(w, h int)             { p.sizes = append(p.sizes, [2]int{w, h}) }
func (p *stubPane) Update(m keys.Msg) tea.Cmd    { p.keys = append(p.keys, m.Seq); return nil }
func (p *stubPane) Render(int, int) string       { return "stub" }
func (p *stubPane) FullHelp() [][]key.Binding {
	return [][]key.Binding{{
		key.NewBinding(key.WithKeys("] h")), key.NewBinding(key.WithKeys("z h")),
	}}
}

func TestSetFileOnChangedMetadata(t *testing.T) {
	h := loaded(t, 100, 30)
	p := &recordPane{}
	h.v.filePane = p
	h.press("j")
	if len(p.files) != 1 {
		t.Fatalf("SetFile calls = %v", p.files)
	}
	d := testDetail()
	d.Files[0].Additions = 99
	h.send(prLoadedMsg{repo: testRepo, number: 7, seq: h.v.seq, detail: d})
	if len(p.files) != 2 || p.files[1].Additions != 99 || p.files[1].Path != p.files[0].Path {
		t.Errorf("pane did not get the new metadata: %+v", p.files)
	}
	h.send(prLoadedMsg{repo: testRepo, number: 7, seq: h.v.seq, detail: d})
	if len(p.files) != 2 {
		t.Errorf("unchanged file repeated: %+v", p.files)
	}
}

type recordPane struct {
	stubPane
	files []github.ChangedFile
}

func (p *recordPane) SetFile(f github.ChangedFile) { p.files = append(p.files, f) }

func TestFilePaneContractThroughRoot(t *testing.T) {
	c := fake.New()
	c.SetPullRequest(testDetail())
	c.SetDiff(testRepo, 7, readDiff(t))
	p := &stubPane{}
	v := New(c, testRepo, 7, WithTheme(theme.New(true)), WithFilePane(p))
	noTick := func(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }
	m := ui.New(v, theme.New(true), ui.Options{Tick: noTick})
	h := &harness{t: t, v: v, client: c}

	step := func(msg tea.Msg) { _, _ = m.Update(msg) }
	keyp := func(k string) { step(tea.KeyPressMsg{Code: rune(k[0]), Text: k}) }
	step(tea.WindowSizeMsg{Width: 100, Height: 25})
	h.run(v.Init())

	if len(p.sizes) == 0 {
		t.Fatal("SetSize was never called")
	}
	last := p.sizes[len(p.sizes)-1]
	step(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := p.sizes[len(p.sizes)-1]; got == last {
		t.Errorf("resize did not change SetSize: %v", got)
	}

	keyp("]")
	keyp("f") // ] f selects the first file
	keyp("l") // focus the right pane
	keyp("]")
	keyp("h")
	keyp("z")
	keyp("h")
	keyp("j")
	if diff := cmp.Diff([]string{"] h", "z h", "j"}, p.keys); diff != "" {
		t.Errorf("keys reaching the pane (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"internal/sync/client.go"}, p.files); diff != "" {
		t.Fatalf("SetFile calls (-want +got):\n%s", diff)
	}

	// Refresh with the same file selected must not call SetFile again.
	h.press("R")
	if len(p.files) != 1 {
		t.Errorf("SetFile called on refresh: %v", p.files)
	}
	if v.selectedPath() != "internal/sync/client.go" {
		t.Errorf("selection lost: %q", v.selectedPath())
	}
}
