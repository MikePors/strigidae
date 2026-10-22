package workspace

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/termtext"
)

// Layout constants.
const (
	narrowBelow = 80 // below this width only one pane is shown
	headerRows  = 2
	filesMin    = 26
	filesMax    = 44
)

// panesHeight is the height available to the panes below the header.
func (v *View) panesHeight() int { return max(0, v.height-headerRows) }

// paneWidths returns the outer widths of the Files and right panes. In a
// narrow terminal the unfocused pane gets zero width.
func (v *View) paneWidths() (files, right int) {
	if v.width < narrowBelow {
		if v.focus == paneFiles {
			return v.width, 0
		}
		return 0, v.width
	}
	files = min(max(v.width*30/100, filesMin), filesMax)
	return files, v.width - files
}

// layout sizes the overview viewport and keeps the Files selection in view.
// It runs on resize, selection change and render.
func (v *View) layout() {
	filesW, rightW := v.paneWidths()
	h := max(0, v.panesHeight()-2)
	v.sizeFilePane(filesW, rightW, h)
	if rightW > 0 {
		v.ov.ensure(v.th, v.md, v.detail, threadsView{v.threads, v.thrLoaded, v.thrPending, v.thrErr}, v.now(), max(1, rightW-4), h)
	}
	v.fw.SetRows(v.entries())
	v.fw.SetSize(h)
}

// sizeFilePane tells the file pane its inner size, also while it is hidden in
// a narrow terminal, whenever that size changes.
func (v *View) sizeFilePane(filesW, rightW, h int) {
	if rightW == 0 {
		rightW = v.width // the pane would get the whole width once shown
		if filesW > 0 && v.width >= narrowBelow {
			rightW = v.width - filesW
		}
	}
	w := max(0, rightW-2)
	if w != v.paneW || h != v.paneH {
		v.paneW, v.paneH = w, h
		v.filePane.SetSize(w, h)
	}
}

// Render implements ui.View.
func (v *View) Render(width, height int) string {
	v.width, v.height = width, height
	if width < 4 || height < headerRows+3 {
		return ""
	}
	v.layout()
	switch {
	case v.detail == nil && v.err != nil:
		return v.renderFailure()
	case v.detail == nil:
		return v.th.Title.Render(v.Title()) + "\n" +
			v.th.Muted.Render(termtext.Clean(fmt.Sprintf("Loading %s#%d…", v.repo, v.number)))
	}

	filesW, rightW := v.paneWidths()
	h := v.panesHeight()
	var left, right []string
	if filesW > 0 {
		left = drawPane(v, "Files", v.fileRows(filesW-2, h-2), filesW, h, v.focus == paneFiles)
	}
	if rightW > 0 {
		right = drawPane(v, v.rightTitle(), v.rightRows(rightW-2, h-2), rightW, h, v.focus == paneRight)
	}
	rows := make([]string, 0, height)
	rows = append(rows, v.renderHeader(width)...)
	for i := range h {
		var l, r string
		if left != nil {
			l = left[i]
		}
		if right != nil {
			r = right[i]
		}
		rows = append(rows, l+r)
	}
	if v.picker != nil {
		overlay(rows, v.picker.Render(width, height-headerRows), width, headerRows)
	}
	if v.chooser != nil {
		overlay(rows, v.chooser.Render(width, height-headerRows), width, headerRows)
	}
	return strings.Join(rows, "\n")
}

func (v *View) renderFailure() string {
	head := v.th.Title.Render(v.Title())
	if v.notFound {
		return head + "\n" +
			v.th.ErrorText.Render(termtext.Clean(fmt.Sprintf("Pull request %s#%d was not found.", v.repo, v.number))) + "\n" +
			v.th.Muted.Render("It may not exist, or you may lack access. Press R to retry.")
	}
	return head + "\n" +
		v.th.ErrorText.Render("Could not load the pull request.") + "\n" +
		v.th.Normal.Render(ansi.Wrap(termtext.SafeError(v.err).Error(), max(1, v.width), "")) + "\n" +
		v.th.Muted.Render("Press R to retry.")
}

func (v *View) renderHeader(width int) []string {
	d := v.detail
	badge := v.stateBadge()
	line1 := badge + " " + v.th.Title.Render(fmt.Sprintf("#%d", d.Number)) + " " + v.th.StrongText.Render(termtext.Clean(d.Title))

	sep := v.th.Muted.Render(" · ")
	parts := []string{
		v.th.Author.Render(termtext.Clean(d.Author)),
		v.th.Muted.Render(termtext.Clean(d.BaseRef) + " ← " + termtext.Clean(d.HeadRef)),
	}
	switch d.ReviewDecision {
	case github.ReviewDecisionApproved:
		parts = append(parts, v.th.FileAdded.Render("approved"))
	case github.ReviewDecisionChangesRequested:
		parts = append(parts, v.th.FileDeleted.Render("changes requested"))
	case github.ReviewDecisionReviewRequired:
		parts = append(parts, v.th.FileModified.Render("review required"))
	case github.ReviewDecisionNone:
	}
	if len(d.Labels) > 0 {
		names := make([]string, len(d.Labels))
		for i, l := range d.Labels {
			names[i] = "[" + termtext.Clean(l.Name) + "]"
		}
		parts = append(parts, v.th.Normal.Render(strings.Join(names, " ")))
	}
	if len(d.ReviewRequests) > 0 {
		names := make([]string, len(d.ReviewRequests))
		for i, r := range d.ReviewRequests {
			names[i] = termtext.Clean(r.Name)
			if r.IsTeam {
				names[i] = "team " + names[i]
			}
		}
		parts = append(parts, v.th.Muted.Render("reviewers: "+strings.Join(names, ", ")))
	}
	line2 := strings.Join(parts, sep)
	return []string{ansi.Truncate(line1, width, "…"), ansi.Truncate(line2, width, "…")}
}

func (v *View) stateBadge() string {
	d := v.detail
	switch {
	case d.State == github.PRStateMerged:
		return v.th.StateMerged.Render("merged")
	case d.State == github.PRStateClosed:
		return v.th.StateClosed.Render("closed")
	case d.Draft:
		return v.th.StateDraft.Render("draft")
	default:
		return v.th.StateOpen.Render("open")
	}
}

func (v *View) rightTitle() string {
	if p := v.selectedPath(); p != "" {
		return termtext.Clean(p)
	}
	return "Overview"
}

// fileRows renders the Files list window of height rows.
func (v *View) fileRows(width, height int) []string {
	files := v.files()
	var rows []string
	v.fw.SetSize(height)
	for i := v.fw.Top(); i < v.entries() && len(rows) < height; i++ {
		selected := i == v.sel()
		if i == 0 {
			rows = append(rows, v.overviewRow(selected))
			continue
		}
		rows = append(rows, v.fileRow(files[i-1], width, selected))
	}
	return rows
}

func (v *View) overviewRow(selected bool) string {
	if selected {
		return v.th.Selected.Render("> Overview")
	}
	return v.th.Normal.Render("  Overview")
}

func (v *View) fileRow(f github.ChangedFile, width int, selected bool) string {
	counts := ""
	if f.Additions > 0 {
		counts += fmt.Sprintf("+%d", f.Additions)
	}
	if f.Deletions > 0 {
		counts += fmt.Sprintf(" -%d", f.Deletions)
	}
	counts = strings.TrimSpace(counts)
	pathW := max(4, width-2-2-1-ansi.StringWidth(counts))
	path := termtext.Clean(f.Path)
	if over := ansi.StringWidth(path) - pathW; over > 0 {
		path = ansi.TruncateLeft(path, over+1, "…") // +1: the ellipsis takes a cell
	}
	path = padRight(path, pathW)
	marker := changeMarker(f.Change)

	if selected {
		return v.th.Selected.Render("> " + marker + " " + path + " " + counts)
	}
	mStyle := v.th.FileModified
	switch f.Change {
	case github.FileAdded:
		mStyle = v.th.FileAdded
	case github.FileDeleted:
		mStyle = v.th.FileDeleted
	case github.FileModified, github.FileRenamed, github.FileCopied, github.FileChanged:
	}
	var cs []string
	if f.Additions > 0 {
		cs = append(cs, v.th.FileAdded.Render(fmt.Sprintf("+%d", f.Additions)))
	}
	if f.Deletions > 0 {
		cs = append(cs, v.th.FileDeleted.Render(fmt.Sprintf("-%d", f.Deletions)))
	}
	return "  " + mStyle.Render(marker) + " " + v.th.Normal.Render(path) + " " + strings.Join(cs, " ")
}

func changeMarker(c github.FileChangeType) string {
	switch c {
	case github.FileAdded:
		return "A"
	case github.FileDeleted:
		return "D"
	case github.FileRenamed:
		return "R"
	case github.FileCopied:
		return "C"
	case github.FileModified:
		return "M"
	case github.FileChanged:
		return "T"
	default:
		return "?"
	}
}

// rightRows renders the right pane's content.
func (v *View) rightRows(width, height int) []string {
	if v.sel() > 0 {
		return strings.Split(v.filePane.Render(width, height), "\n")
	}
	lines := strings.Split(v.ov.vp.View(), "\n")
	for i, l := range lines {
		lines[i] = " " + l
	}
	return lines
}

func padRight(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// drawPane frames lines in a titled rounded box of exactly w x h cells.
func drawPane(v *View, title string, lines []string, w, h int, focused bool) []string {
	border, titleStyle := v.th.PaneBorder, v.th.PaneTitle
	if focused {
		border, titleStyle = v.th.PaneBorderFocus, v.th.PaneTitleFocus
	}
	inner := w - 2
	title = ansi.Truncate(title, max(0, inner-3), "…")
	t := " " + title + " "
	top := border.Render("╭─") + titleStyle.Render(t) +
		border.Render(strings.Repeat("─", max(0, inner-1-ansi.StringWidth(t)))+"╮")
	out := make([]string, 0, h)
	out = append(out, top)
	side := border.Render("│")
	for i := range h - 2 {
		var l string
		if i < len(lines) {
			l = lines[i]
		}
		l = padRight(ansi.Truncate(l, inner, ""), inner)
		out = append(out, side+l+side)
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// overlay draws box centred horizontally over rows, starting at row top.
func overlay(rows []string, box string, width, top int) {
	if box == "" {
		return
	}
	bl := strings.Split(box, "\n")
	bw := ansi.StringWidth(bl[0])
	x := max(0, (width-bw)/2)
	for i, l := range bl {
		r := top + 1 + i
		if r >= len(rows) {
			return
		}
		base := padRight(rows[r], width)
		rows[r] = ansi.Cut(base, 0, x) + l + ansi.Cut(base, x+bw, width)
	}
}
