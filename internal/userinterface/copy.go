package userinterface

import (
	"errors"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/clipboard"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// CopyText sends text to the clipboard through OSC 52 and, when copier is not
// nil, through the system clipboard tool too: OSC 52 works over SSH but tmux
// and some terminals drop it. A failing tool is only logged, because OSC 52
// may have worked. notice is shown in the status bar and must already be safe
// to draw.
func CopyText(copier clipboard.Copier, timeouts Timeouts, text, notice string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), copyToTool(copier, timeouts, text), Info(notice))
}

// CopyURL copies the URL of a pull request like CopyText. An empty URL is an
// error, and so is one with control characters, so a pasted escape or line
// break can never reach a shell or a chat as input.
func CopyURL(copier clipboard.Copier, timeouts Timeouts, url string) tea.Cmd {
	if url == "" {
		return Error(errors.New("no pull request URL to copy"))
	}
	if terminaltext.Clean(url) != url {
		return Error(fmt.Errorf("not copied: %s contains control characters", terminaltext.Clean(url)))
	}
	return CopyText(copier, timeouts, url, "Copied "+terminaltext.Clean(url))
}

// copyToTool pipes text to the system clipboard tool, if there is a copier.
func copyToTool(copier clipboard.Copier, timeouts Timeouts, text string) tea.Cmd {
	if copier == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := timeouts.RequestContext()
		defer cancel()
		if err := copier.Copy(ctx, text); err != nil && !errors.Is(err, clipboard.ErrNoTool) {
			slog.Warn("copy with clipboard tool", "err", err)
		}
		return nil
	}
}
