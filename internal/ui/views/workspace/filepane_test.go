package workspace

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// stubPane records what the workspace tells a FilePane.
type stubPane struct {
	files []string
	sizes [][2]int
	keys  []string
}

func (p *stubPane) SetTheme(theme.Theme) {}

func (p *stubPane) CommentRange() (LineRange, bool) { return LineRange{}, false }
func (p *stubPane) VisualStart() bool               { return false }
func (p *stubPane) VisualActive() bool              { return false }
func (p *stubPane) VisualMove(int) error            { return nil }
func (p *stubPane) VisualEnd()                      {}

func (p *stubPane) SetDiff(DiffState) {}

func (p *stubPane) SetThreads([]github.ReviewThread)      {}
func (p *stubPane) FocusedThreads() []github.ReviewThread { return nil }

func (p *stubPane) SetFile(f github.ChangedFile) { p.files = append(p.files, f.Path) }

func (p *stubPane) SetSize(w, h int) { p.sizes = append(p.sizes, [2]int{w, h}) }

func (p *stubPane) Update(m keys.Msg) tea.Cmd { p.keys = append(p.keys, m.Seq); return nil }

func (p *stubPane) Render(int, int) string { return "stub" }

func (p *stubPane) FullHelp() [][]key.Binding {
	return [][]key.Binding{{
		key.NewBinding(key.WithKeys("]")), key.NewBinding(key.WithKeys("z h")),
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
	h.send(prLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: h.v.seq, detail: d})
	if len(p.files) != 2 || p.files[1].Additions != 99 || p.files[1].Path != p.files[0].Path {
		t.Errorf("pane did not get the new metadata: %+v", p.files)
	}
	h.send(prLoadedMsg{owner: h.v, repo: testRepo, number: 7, seq: h.v.seq, detail: d})
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

	keyp("}") // selects the first file
	keyp("l") // focus the right pane
	keyp("]")
	keyp("z")
	keyp("h")
	keyp("j")
	if diff := cmp.Diff([]string{"]", "z h", "j"}, p.keys); diff != "" {
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
