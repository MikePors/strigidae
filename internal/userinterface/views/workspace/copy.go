package workspace

import (
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/clipboard"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// copyFilename copies the selected file's path to the clipboard. The Files
// pane cursor and the diff pane's file are the same selection, so focus does
// not matter.
func (view *View) copyFilename() tea.Cmd {
	path := view.selectedPath()
	if path == "" {
		return userinterface.Error(errors.New("no file to copy"))
	}
	// Refuse control and bidi characters so the path cannot carry escapes or
	// line breaks (a pasted newline would submit a line). It is the raw path,
	// so it may still contain shell metacharacters.
	if terminaltext.Clean(path) != path {
		return userinterface.Error(fmt.Errorf("not copied: %s contains control characters", terminaltext.Clean(path)))
	}
	return view.copyText(path, "Copied "+terminaltext.Clean(path))
}

// copyText sends text to the clipboard through OSC 52 and, when there is a
// clipboard tool, through that too: OSC 52 works over SSH but tmux and some
// terminals drop it. A failing tool is only logged, because OSC 52 may have
// worked.
func (view *View) copyText(text, notice string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), view.copyToTool(text), userinterface.Info(notice))
}

// copyToTool pipes text to the system clipboard tool, if the view has a copier.
func (view *View) copyToTool(text string) tea.Cmd {
	copier := view.clipboard
	if copier == nil {
		return nil
	}
	timeouts := view.timeouts
	return func() tea.Msg {
		ctx, cancel := timeouts.RequestContext()
		defer cancel()
		if err := copier.Copy(ctx, text); err != nil && !errors.Is(err, clipboard.ErrNoTool) {
			slog.Warn("copy with clipboard tool", "err", err)
		}
		return nil
	}
}
