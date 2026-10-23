package workspace

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func TestThemeMessageRebuilds(test *testing.T) {
	harness := loaded(test, 100, 24)
	dark := harness.render()
	harness.send(userinterface.ThemeMessage{Theme: theme.New(false)})
	if harness.render() == dark {
		test.Error("light theme should change the rendering")
	}
}

func TestRendersWithoutPanicAtTinySizes(test *testing.T) {
	for _, size := range [][2]int{{0, 0}, {3, 3}, {10, 4}, {20, 6}, {79, 5}, {80, 5}} {
		harness := loaded(test, size[0], size[1])
		harness.press("f", "j")
		_ = harness.render()
		harness.press("l")
		_ = harness.render()
	}
}

// fixtures for the golden tests.

func TestGolden(test *testing.T) {
	test.Run("overview", func(test *testing.T) {
		harness := loaded(test, 100, 40)
		golden.Assert(test, "overview", harness.render(), false)
	})
	test.Run("overview_scrolled_to_conversation", func(test *testing.T) {
		harness := loaded(test, 100, 24)
		harness.press("l", "]")
		golden.Assert(test, "overview_conversation", harness.render(), false)
	})
	test.Run("file_selected", func(test *testing.T) {
		harness := loaded(test, 100, 24)
		harness.press("}", "}", "l")
		golden.Assert(test, "file_selected", harness.render(), false)
	})
	test.Run("picker", func(test *testing.T) {
		harness := loaded(test, 100, 24)
		harness.press("f")
		harness.typ("retry")
		golden.Assert(test, "picker", harness.render(), false)
	})
	test.Run("narrow_files", func(test *testing.T) {
		harness := loaded(test, 60, 20)
		harness.press("j")
		golden.Assert(test, "narrow_files", harness.render(), false)
	})
	test.Run("narrow_right", func(test *testing.T) {
		harness := loaded(test, 60, 20)
		harness.press("enter")
		golden.Assert(test, "narrow_right", harness.render(), false)
	})
	test.Run("loading", func(test *testing.T) {
		harness := newHarness(test, 80, 10)
		golden.Assert(test, "loading", harness.render(), false)
	})
	test.Run("error", func(test *testing.T) {
		harness := newHarness(test, 80, 10)
		harness.client.SetError(fake.MethodGetPullRequest, errors.New("load pull request: connection refused"))
		harness.run(harness.view.Init())
		golden.Assert(test, "error", harness.render(), false)
	})
	test.Run("not_found", func(test *testing.T) {
		harness := newHarness(test, 80, 10)
		harness.view.number = 99
		harness.run(harness.view.Init())
		golden.Assert(test, "not_found", harness.render(), false)
	})
	test.Run("no_files_no_description", func(test *testing.T) {
		detail := testDetail()
		detail.Body, detail.Files, detail.Comments, detail.Reviews = "", nil, nil, nil
		client := fake.New()
		client.SetPullRequest(detail)
		client.SetDiff(testRepository, 7, readDiff(test))
		view := New(client, testRepository, 7, WithClock(func() time.Time { return testNow }), WithTheme(theme.New(true)))
		testHarness := &harness{test: test, view: view, client: client}
		testHarness.send(tea.WindowSizeMsg{Width: 80, Height: 12})
		testHarness.run(view.Init())
		testHarness.press("}", "f")
		golden.Assert(test, "empty", testHarness.render(), false)
	})
}

func TestLoadingLineCleansRepository(test *testing.T) {
	view := New(fake.New(), github.Repository{Owner: "o\x1b[2J\x1b]52;c;aGk=\x07", Name: "n\u202e"}, 7, WithTheme(theme.New(true)))
	view.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	assertTame(test, "loading line", view.Render(80, 10))
}
