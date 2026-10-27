package diffview

import (
	"slices"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// placeholderText starts the one row left of a hidden hunk, before its header.
const placeholderText = "▸ reviewed hunk hidden  "

// SetHunkHidden sets how the pane learns which hunks to hide: hidden reports
// it for a hunk of a file, by index into file.Hunks. A hidden hunk is drawn as
// one placeholder row; its lines and the threads on them are left out, and
// NextHunk and PreviousHunk skip it. The rows are rebuilt from it by SetFile
// and HiddenHunksChanged only. nil hides none.
func (model *Model) SetHunkHidden(hidden func(file *diff.File, hunk int) bool) {
	model.hunkHidden = hidden
	model.layout()
}

// HiddenHunksChanged rebuilds the rows after the answers of the SetHunkHidden
// function changed. A cursor inside a hunk that is now hidden moves to its
// placeholder.
func (model *Model) HiddenHunksChanged() { model.layout() }

// SetHunkFade sets how the pane learns how far a hunk has faded: fade
// returns 0 for its normal look, up to theme.FadeSteps for the faintest. It
// is asked while rendering, so a change shows on the next render. The cursor
// row never fades. nil fades none.
func (model *Model) SetHunkFade(fade func(file *diff.File, hunk int) int) {
	model.hunkFade = fade
}

func (model *Model) isHidden(hunk int) bool {
	return model.hunkHidden != nil && model.file != nil && model.hunkHidden(model.file, hunk)
}

// placeholderRow returns the row of the placeholder of hunk, or -1.
func (model *Model) placeholderRow(hunk int) int {
	for i, row := range model.rows {
		if row.placeholder && row.hunk == hunk {
			return i
		}
	}
	return -1
}

func (model *Model) renderPlaceholder(i int, row row, current int) string {
	mark := " "
	if current == 1 {
		mark = ">"
	}
	header, _ := displayText(model.file.Hunks[row.hunk].Header()) // file content
	text := ansi.Truncate(mark+model.relativeCell(i)+placeholderText+header, model.width, "")
	text += spaces(model.width - ansi.StringWidth(text))
	return model.styles.placeholder[current].Render(text)
}

// fadeOf returns how far row i has faded: its hunk's fade, for the hunk's
// header, lines and the threads on them, or a thread row's own fade (see
// SetThreadFade) when that is further. Notices and the cursor row never fade,
// and the threads after the diff fade only by their own; a file with no hunks
// has only those. (A placeholder's hunk has stopped fading.)
func (model *Model) fadeOf(i int) int {
	row := model.rows[i]
	if i == model.window.Cursor() || row.message {
		return 0
	}
	fade := 0
	if row.threadRow != nil {
		fade = model.threadFadeStep(row.threadRow.threadIndex)
	}
	if model.hunkFade == nil {
		return fade
	}
	if row.threadRow != nil && slices.Contains(model.orphans, row.threadRow.threadIndex) {
		return fade
	}
	hunk := row.hunk
	if row.threadRow != nil {
		hunk = model.base[row.baseIndex].hunk
	}
	return max(fade, min(model.hunkFade(model.file, hunk), theme.FadeSteps))
}

// faded draws already rendered lines in fade step fade: their text only, in
// the fading colour.
func (model *Model) faded(lines []string, fade int) []string {
	style := model.theme.Fade[fade-1]
	for k, line := range lines {
		lines[k] = style.Render(ansi.Strip(line))
	}
	return lines
}
