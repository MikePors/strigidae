package workspace

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/github"
)

// With the hideviewed option the Files pane lists fewer rows than there are
// entries. view.rows says which: the entry (0 is Overview, i is Files[i-1])
// shown on each row, ascending. The window's cursor is a row; view.current is
// an entry, the one whose content the right pane shows. They differ when the
// file picker opened a file that is hidden: the cursor then waits on the
// nearest shown row.

const fadeKeyPrefix = "viewed:"

func fadeKey(path string) string { return fadeKeyPrefix + path }

// hidden reports whether the Files pane leaves file out: viewed, the option
// on, and not in its grace period.
func (view *View) hidden(file github.ChangedFile) bool {
	return view.options.HideViewed() && file.Viewed == github.FileViewed && !view.fades.active(fadeKey(file.Path))
}

// shownEntries returns the entries the Files pane lists.
func (view *View) shownEntries() []int {
	rows := []int{0}
	for i, file := range view.files() {
		if !view.hidden(file) {
			rows = append(rows, i+1)
		}
	}
	return rows
}

// nearestRow returns the row showing the entry closest to entry; between two
// equally close ones, the later.
func nearestRow(rows []int, entry int) int {
	distance := func(candidate int) int { return max(candidate-entry, entry-candidate) }
	best := 0
	for row, candidate := range rows {
		if distance(candidate) <= distance(rows[best]) {
			best = row
		}
	}
	return best
}

// cursorEntry is the entry on the cursor's row.
func (view *View) cursorEntry() int {
	if cursor := view.window.Cursor(); cursor < len(view.rows) {
		return view.rows[cursor]
	}
	return 0
}

// place lists the rows anew and puts the cursor on the one nearest entry.
func (view *View) place(entry int) {
	rows := view.shownEntries()
	changed := !slices.Equal(rows, view.rows)
	view.rows = rows
	view.window.SetRows(len(view.rows))
	view.window.Restore(nearestRow(view.rows, entry), view.window.Top(), 0)
	if changed {
		view.search.Refresh(view.searchList()) // its rows are these
	}
}

// reflow is place for a change of what is hidden: the cursor stays on its
// file, or moves to the nearest one when that file went. The right pane
// follows the cursor off a file that went, unless it was showing another.
func (view *View) reflow() {
	anchor := view.cursorEntry()
	view.place(anchor)
	if view.current == anchor && view.cursorEntry() != anchor {
		view.current = view.cursorEntry()
		view.selectionChanged()
	}
}

// cursorMoved makes the right pane follow the cursor after the window moved it.
func (view *View) cursorMoved() {
	if entry := view.cursorEntry(); entry != view.current {
		view.current = entry
		view.selectionChanged()
	}
}

// moveOffMarked moves the cursor to the row after the current one, or the one
// before it at the end of the list.
func (view *View) moveOffMarked() {
	row := nearestRow(view.rows, view.current)
	next := row + 1
	if next >= len(view.rows) {
		next = row - 1
	}
	if next < 0 {
		return
	}
	view.window.GotoRow(next)
	view.cursorMoved()
}

// startFade keeps path's row for the grace period and asks for its timers.
func (view *View) startFade(path string) tea.Cmd {
	key := fadeKey(path)
	return view.fadeCommand(key, view.fades.begin(key, view.now()))
}

// fadeTicked handles a timer of a fade.
func (view *View) fadeTicked(message fadeMessage) tea.Cmd {
	path, ok := strings.CutPrefix(message.key, fadeKeyPrefix)
	if !ok {
		return nil
	}
	current, over := view.fades.advance(message.key, message.generation, view.now())
	switch {
	case !current:
		return nil
	case over:
		view.reflow()
		return nil
	case !view.stillViewed(path):
		view.fades.cancel(message.key)
		return nil
	}
	return view.fadeCommand(message.key, message.generation)
}

func (view *View) stillViewed(path string) bool {
	for _, file := range view.files() {
		if file.Path == path {
			return file.Viewed == github.FileViewed
		}
	}
	return false
}

// optionsChanged applies the hideviewed option to the rows at once. Turning it
// off ends the fades; the files stay viewed.
func (view *View) optionsChanged() {
	if !view.options.HideViewed() {
		view.fades.clear()
	}
	view.reflow()
}
