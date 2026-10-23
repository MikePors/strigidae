package picker

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

func typ(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestFilterAndChoose(t *testing.T) {
	m := New(theme.Default(), "Files", []string{"a/b.go", "c/d.go", "README.md"})
	if m.Matches() != 3 {
		t.Fatalf("matches = %d", m.Matches())
	}
	typ(m, "DGO") // case-insensitive subsequence
	if m.Matches() != 1 {
		t.Fatalf("matches for %q = %d", m.Query(), m.Matches())
	}
	res := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if res.Outcome != Chosen || res.Index != 1 {
		t.Errorf("result = %+v, want chosen index 1", res)
	}
}

func TestCursorAndEsc(t *testing.T) {
	m := New(theme.Default(), "Files", []string{"x", "y", "z"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // clamps
	if res := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); res.Index != 2 {
		t.Errorf("index = %d, want 2", res.Index)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if res := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); res.Index != 1 {
		t.Errorf("index = %d, want 1", res.Index)
	}
	if res := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); res.Outcome != Cancelled {
		t.Errorf("esc outcome = %v", res.Outcome)
	}
}

func TestRenderFitsAndHandlesNoRoom(t *testing.T) {
	m := New(theme.Default(), "Files", []string{"one", "two"})
	out := ansi.Strip(m.Render(40, 10))
	if out == "" {
		t.Fatal("expected a box")
	}
	if got := m.Render(5, 3); got != "" {
		t.Errorf("no room should render nothing, got %q", got)
	}
	typ(m, "zzz")
	if ansi.Strip(m.Render(40, 10)) == out {
		t.Error("filter should change the rendering")
	}
}
