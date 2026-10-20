package placeholder

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/MikePors/strigidae/internal/golden"
	"github.com/MikePors/strigidae/internal/ui"
	"github.com/MikePors/strigidae/internal/ui/keys"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

func sized() *View {
	v := New(theme.Default(), 1)
	v.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	return v
}

func send(v *View, seqs ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, s := range seqs {
		_, cmd = v.Update(keys.Msg{Seq: s})
	}
	return cmd
}

func TestNavigation(t *testing.T) {
	// List height is 8 (10 rows minus two header rows).
	tests := []struct {
		name       string
		keys       []string
		wantCursor int
		wantTop    int
	}{
		{"down", []string{"j", "j"}, 2, 0},
		{"up clamps at top", []string{"k"}, 0, 0},
		{"bottom", []string{"G"}, lineCount - 1, lineCount - 8},
		{"top after bottom", []string{"G", "g g"}, 0, 0},
		{"half page", []string{"ctrl+d"}, 4, 0},
		{"page down scrolls", []string{"ctrl+f"}, 8, 1},
		{"page up", []string{"ctrl+f", "ctrl+f", "ctrl+b"}, 8, 8},
		{"half up", []string{"ctrl+d", "ctrl+d", "ctrl+u"}, 4, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := sized()
			send(v, tt.keys...)
			if v.cursor != tt.wantCursor || v.top != tt.wantTop {
				t.Errorf("cursor/top = %d/%d, want %d/%d", v.cursor, v.top, tt.wantCursor, tt.wantTop)
			}
		})
	}
}

func TestEnterPushesNextScreen(t *testing.T) {
	v := sized()
	cmd := send(v, "enter")
	push, ok := cmd().(ui.PushMsg)
	if !ok {
		t.Fatalf("enter returned %T, want ui.PushMsg", cmd())
	}
	if diff := cmp.Diff("screen 2", push.View.Title()); diff != "" {
		t.Errorf("title mismatch (-want +got):\n%s", diff)
	}
}

func TestRefreshShowsLoading(t *testing.T) {
	v := sized()
	if cmd := send(v, "R"); cmd == nil {
		t.Fatal("R should start a refresh")
	}
	_, cmd := v.Update(refreshedMsg{})
	if cmd == nil {
		t.Error("refreshedMsg should clear loading and show info")
	}
}

func TestRender(t *testing.T) {
	v := sized()
	golden.Assert(t, "initial", v.Render(50, 10), false)

	send(v, "ctrl+f", "j")
	golden.Assert(t, "scrolled", v.Render(50, 10), false)

	deep := New(theme.Default(), 3)
	golden.Assert(t, "depth3_narrow", deep.Render(30, 5), false)
}

func TestRenderTiny(t *testing.T) {
	if got := New(theme.Default(), 1).Render(10, 1); got != "" {
		t.Errorf("Render at height 1 = %q, want empty", got)
	}
}
