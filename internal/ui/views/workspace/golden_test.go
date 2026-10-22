package workspace

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

func TestThemeMessageRebuilds(t *testing.T) {
	h := loaded(t, 100, 24)
	dark := h.render()
	h.send(ui.ThemeMsg{Theme: theme.New(false)})
	if h.render() == dark {
		t.Error("light theme should change the rendering")
	}
}

func TestRendersWithoutPanicAtTinySizes(t *testing.T) {
	for _, sz := range [][2]int{{0, 0}, {3, 3}, {10, 4}, {20, 6}, {79, 5}, {80, 5}} {
		h := loaded(t, sz[0], sz[1])
		h.press("f", "j")
		_ = h.render()
		h.press("l")
		_ = h.render()
	}
}

// fixtures for the golden tests.

func TestGolden(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		h := loaded(t, 100, 40)
		golden.Assert(t, "overview", h.render(), false)
	})
	t.Run("overview_scrolled_to_conversation", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("l", "]")
		golden.Assert(t, "overview_conversation", h.render(), false)
	})
	t.Run("file_selected", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("}", "}", "l")
		golden.Assert(t, "file_selected", h.render(), false)
	})
	t.Run("picker", func(t *testing.T) {
		h := loaded(t, 100, 24)
		h.press("f")
		h.typ("retry")
		golden.Assert(t, "picker", h.render(), false)
	})
	t.Run("narrow_files", func(t *testing.T) {
		h := loaded(t, 60, 20)
		h.press("j")
		golden.Assert(t, "narrow_files", h.render(), false)
	})
	t.Run("narrow_right", func(t *testing.T) {
		h := loaded(t, 60, 20)
		h.press("enter")
		golden.Assert(t, "narrow_right", h.render(), false)
	})
	t.Run("loading", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		golden.Assert(t, "loading", h.render(), false)
	})
	t.Run("error", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		h.client.SetError(fake.MethodGetPullRequest, errors.New("load pull request: connection refused"))
		h.run(h.v.Init())
		golden.Assert(t, "error", h.render(), false)
	})
	t.Run("not_found", func(t *testing.T) {
		h := newHarness(t, 80, 10)
		h.v.number = 99
		h.run(h.v.Init())
		golden.Assert(t, "not_found", h.render(), false)
	})
	t.Run("no_files_no_description", func(t *testing.T) {
		d := testDetail()
		d.Body, d.Files, d.Comments, d.Reviews = "", nil, nil, nil
		c := fake.New()
		c.SetPullRequest(d)
		c.SetDiff(testRepo, 7, readDiff(t))
		v := New(c, testRepo, 7, WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true)))
		h := &harness{t: t, v: v, client: c}
		h.send(tea.WindowSizeMsg{Width: 80, Height: 12})
		h.run(v.Init())
		h.press("}", "f")
		golden.Assert(t, "empty", h.render(), false)
	})
}

func TestLoadingLineCleansRepo(t *testing.T) {
	v := New(fake.New(), github.Repo{Owner: "o\x1b[2J\x1b]52;c;aGk=\x07", Name: "n\u202e"}, 7, WithTheme(theme.New(true)))
	v.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	assertTame(t, "loading line", v.Render(80, 10))
}
