package workspace

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/keys"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// DiffState is the workspace's diff and where loading it stands.
type DiffState struct {
	// Raw is the unified diff of the whole pull request. On a refresh it keeps
	// the previous text while Loading is set, and after a failed refresh.
	Raw string
	// Parsed is Raw parsed by internal/diff, done in the load command. It is nil
	// until a load gets as far as parsing. When ParseError is set it holds the
	// files parsed before the problem: look files up in it as usual and treat a
	// missing one as "no diff available".
	Parsed *diff.Diff
	// ParseError is why Raw could not be parsed completely. The workspace reports
	// it once on the status bar; Parsed is still usable.
	ParseError error
	// Err is why the diff could not be loaded at all, for example
	// github.ErrDiffTooLarge. Raw and Parsed are empty then. A failed refresh
	// does not set it: the previous diff is kept instead.
	Err error
	// Loading is set while a (re)load is in flight.
	Loading bool
}

// LineRange is the lines of a file a review comment is written on. Start and
// End are line numbers on Side and are equal for a single line.
type LineRange struct {
	Path       string
	Side       github.DiffSide
	Start, End int
}

// FilePane is the content of the right pane while a file is selected. The
// diff component plugs in here through WithFilePane.
//
// Contract notes for implementers:
//   - SetDiff may be called again with an identical Raw (once when a load
//     starts, with Loading set, once when it ends, and on every refresh). Cache
//     parsing by Raw. A failed refresh keeps the previous Raw and Err stays nil.
//   - SetFile is called when the selected file differs from the last one
//     given (a different path, or the same path with new metadata after a
//     refresh); an unchanged file is not repeated. A repeat call for the same
//     path means only the metadata changed: keep cursor and scroll, and
//     re-resolve the file against the newest SetDiff.
//   - ChangedFile.Path is the new path, or the old path for a deleted file:
//     look files up by new path and fall back to the old one.
//   - SetSize is called whenever the pane's inner size changes (and before the
//     first Render), so Update can use it for page scrolling.
//
// The workspace calls SetTheme, SetDiff, SetFile and SetSize outside of rendering,
// forwards every keys.Msg it does not handle itself (movement, scrolling and
// the pane's own sequences) to Update while the right pane has focus, and
// calls Render with the pane's inner size. FullHelp must list every binding
// Update reacts to, so multi-key sequences are recognised.
type FilePane interface {
	// SetTheme is called on construction and whenever the theme changes.
	SetTheme(activeTheme theme.Theme)
	// SetDiff is called when a diff load starts and when it finishes.
	SetDiff(diffState DiffState)
	// SetThreads is called when the pull request's review threads load or
	// reload; the pane shows the ones on its file.
	SetThreads(threads []github.ReviewThread)
	// SetFile is called when the selected file changes, and again when the
	// same path comes back with different metadata (change type, counts)
	// after a refresh. A repeat call for the same path keeps cursor and scroll.
	SetFile(file github.ChangedFile)
	// SetSize gives the pane's inner size; it is called whenever it changes.
	SetSize(width, height int)
	// FocusedThreads returns the review threads at the pane's cursor: the one
	// whose rows it is in, or those on the line it is on. Panes without
	// threads return nil.
	FocusedThreads() []github.ReviewThread
	// CommentRange returns the lines a new comment would go on: the visual
	// selection, else the cursor line. ok is false when the cursor is not on a
	// diff line. Panes without lines return false.
	CommentRange() (lineRange LineRange, ok bool)
	// VisualStart begins a line selection at the cursor, reporting false when
	// it cannot (no diff line under the cursor).
	VisualStart() bool
	// VisualActive reports whether a line selection is in progress.
	VisualActive() bool
	// VisualMove extends the selection by delta lines; a refused move (outside
	// one hunk or one side) leaves it unchanged and returns the reason.
	VisualMove(delta int) error
	// VisualEnd leaves the line selection.
	VisualEnd()
	// Update handles a key while the right pane has focus.
	Update(message keys.Message) tea.Cmd
	// Render draws the pane in exactly width columns and at most height rows.
	Render(width, height int) string
	// FullHelp lists the pane's bindings for the help overlay and sequence matching.
	FullHelp() [][]key.Binding
}
