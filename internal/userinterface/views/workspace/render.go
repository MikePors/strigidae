package workspace

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
)

// Layout constants.
const (
	narrowBelow  = 80 // below this width only one pane is shown
	headerRows   = 2
	filesMinimum = 26
	filesMaximum = 44
)

// panesHeight is the height available to the panes below the header.
func (view *View) panesHeight() int { return max(0, view.height-headerRows) }

// paneWidths returns the outer widths of the Files and right panes. In a
// narrow terminal the unfocused pane gets zero width.
func (view *View) paneWidths() (files, right int) {
	if view.width < narrowBelow {
		if view.focus == paneFiles {
			return view.width, 0
		}
		return 0, view.width
	}
	files = min(max(view.width*30/100, filesMinimum), filesMaximum)
	return files, view.width - files
}

// layout sizes the overview viewport and keeps the Files selection in view.
// It runs on resize, selection change and render.
func (view *View) layout() {
	filesWidth, rightWidth := view.paneWidths()
	height := max(0, view.panesHeight()-2)
	view.sizeFilePane(filesWidth, rightWidth, height)
	if rightWidth > 0 {
		view.ov.ensure(view.th, view.md, view.detail, threadsView{view.threads, view.thrLoaded, view.thrPending, view.thrErr}, view.now(), max(1, rightWidth-4), height)
	}
	view.fw.SetRows(view.entries())
	view.fw.SetSize(height)
}

// sizeFilePane tells the file pane its inner size, also while it is hidden in
// a narrow terminal, whenever that size changes.
func (view *View) sizeFilePane(filesWidth, rightWidth, height int) {
	if rightWidth == 0 {
		rightWidth = view.width // the pane would get the whole width once shown
		if filesWidth > 0 && view.width >= narrowBelow {
			rightWidth = view.width - filesWidth
		}
	}
	width := max(0, rightWidth-2)
	if width != view.paneWidth || height != view.paneHeight {
		view.paneWidth, view.paneHeight = width, height
		view.filePane.SetSize(width, height)
	}
}

// Render implements userinterface.View.
func (view *View) Render(width, height int) string {
	view.width, view.height = width, height
	if width < 4 || height < headerRows+3 {
		return ""
	}
	view.layout()
	switch {
	case view.detail == nil && view.err != nil:
		return view.renderFailure()
	case view.detail == nil:
		return view.th.Title.Render(view.Title()) + "\n" +
			view.th.Muted.Render(terminaltext.Clean(fmt.Sprintf("Loading %s#%d…", view.repository, view.number)))
	}

	filesWidth, rightWidth := view.paneWidths()
	lineHeight := view.panesHeight()
	var left, right []string
	if filesWidth > 0 {
		left = drawPane(view, view.filesTitle(), view.fileRows(filesWidth-2, lineHeight-2), filesWidth, lineHeight, view.focus == paneFiles)
	}
	if rightWidth > 0 {
		right = drawPane(view, view.rightTitle(), view.rightRows(rightWidth-2, lineHeight-2), rightWidth, lineHeight, view.focus == paneRight)
	}
	rows := make([]string, 0, height)
	rows = append(rows, view.renderHeader(width)...)
	for i := range lineHeight {
		var line, text string
		if left != nil {
			line = left[i]
		}
		if right != nil {
			text = right[i]
		}
		rows = append(rows, line+text)
	}
	if view.picker != nil {
		overlay(rows, view.picker.Render(width, height-headerRows), width, headerRows)
	}
	if view.review.chooser != nil {
		overlay(rows, view.review.chooser.Render(width, height-headerRows), width, headerRows)
	}
	return strings.Join(rows, "\n")
}

func (view *View) renderFailure() string {
	head := view.th.Title.Render(view.Title())
	if view.notFound {
		return head + "\n" +
			view.th.ErrorText.Render(terminaltext.Clean(fmt.Sprintf("Pull request %s#%d was not found.", view.repository, view.number))) + "\n" +
			view.th.Muted.Render("It may not exist, or you may lack access. Press R to retry.")
	}
	return head + "\n" +
		view.th.ErrorText.Render("Could not load the pull request.") + "\n" +
		view.th.Normal.Render(ansi.Wrap(terminaltext.SafeError(view.err).Error(), max(1, view.width), "")) + "\n" +
		view.th.Muted.Render("Press R to retry.")
}

func (view *View) renderHeader(width int) []string {
	detail := view.detail
	badge := view.stateBadge()
	line1 := badge + " " + view.th.Title.Render(fmt.Sprintf("#%d", detail.Number)) + " " + view.th.StrongText.Render(terminaltext.Clean(detail.Title))

	separator := view.th.Muted.Render(" · ")
	parts := []string{
		view.th.Author.Render(terminaltext.Clean(detail.Author)),
	}
	switch detail.Checks {
	case github.ChecksPassing:
		parts = append(parts, view.th.PullRequestList.ChecksPassing.Render("✓ checks passing"))
	case github.ChecksFailing:
		parts = append(parts, view.th.PullRequestList.ChecksFailing.Render("✗ checks failing"))
	case github.ChecksRunning:
		parts = append(parts, view.th.PullRequestList.ChecksRunning.Render("● checks running"))
	case github.ChecksNone:
	}
	switch detail.ReviewDecision {
	case github.ReviewDecisionApproved:
		parts = append(parts, view.th.FileAdded.Render("approved"))
	case github.ReviewDecisionChangesRequested:
		parts = append(parts, view.th.FileDeleted.Render("changes requested"))
	case github.ReviewDecisionReviewRequired:
		parts = append(parts, view.th.FileModified.Render("review required"))
	case github.ReviewDecisionNone:
	}
	// Branches are the least glanceable part, so they come after the states
	// and are what truncation eats first.
	parts = append(parts, view.th.Muted.Render(terminaltext.Clean(detail.BaseBranch)+" ← "+terminaltext.Clean(detail.HeadBranch)))
	if len(detail.Labels) > 0 {
		names := make([]string, len(detail.Labels))
		for i, label := range detail.Labels {
			names[i] = "[" + terminaltext.Clean(label.Name) + "]"
		}
		parts = append(parts, view.th.Normal.Render(strings.Join(names, " ")))
	}
	if len(detail.ReviewRequests) > 0 {
		names := make([]string, len(detail.ReviewRequests))
		for i, review := range detail.ReviewRequests {
			names[i] = terminaltext.Clean(review.Name)
			if review.IsTeam {
				names[i] = "team " + names[i]
			}
		}
		parts = append(parts, view.th.Muted.Render("reviewers: "+strings.Join(names, ", ")))
	}
	line2 := strings.Join(parts, separator)
	return []string{ansi.Truncate(line1, width, "…"), ansi.Truncate(line2, width, "…")}
}

func (view *View) stateBadge() string {
	detail := view.detail
	switch {
	case detail.State == github.PullRequestStateMerged:
		return view.th.StateMerged.Render("merged")
	case detail.State == github.PullRequestStateClosed:
		return view.th.StateClosed.Render("closed")
	case detail.Draft:
		return view.th.StateDraft.Render("draft")
	default:
		return view.th.StateOpen.Render("open")
	}
}

func (view *View) rightTitle() string {
	if path := view.selectedPath(); path != "" {
		return terminaltext.Clean(path)
	}
	return "Overview"
}

// fileRows renders the Files list window of height rows.
func (view *View) fileRows(width, height int) []string {
	files := view.files()
	var rows []string
	view.fw.SetSize(height)
	for i := view.fw.Top(); i < view.entries() && len(rows) < height; i++ {
		selected := i == view.selection()
		if i == 0 {
			rows = append(rows, view.overviewRow(selected))
			continue
		}
		rows = append(rows, view.fileRow(files[i-1], width, selected))
	}
	return rows
}

func (view *View) overviewRow(selected bool) string {
	if selected {
		return view.th.Selected.Render("> Overview")
	}
	return view.th.Normal.Render("  Overview")
}

func (view *View) fileRow(file github.ChangedFile, width int, selected bool) string {
	counts := ""
	if file.Additions > 0 {
		counts += fmt.Sprintf("+%d", file.Additions)
	}
	if file.Deletions > 0 {
		counts += fmt.Sprintf(" -%d", file.Deletions)
	}
	counts = strings.TrimSpace(counts)
	pathWidth := max(4, width-2-2-2-1-ansi.StringWidth(counts))
	path := terminaltext.Clean(file.Path)
	if over := ansi.StringWidth(path) - pathWidth; over > 0 {
		path = ansi.TruncateLeft(path, over+1, "…") // +1: the ellipsis takes a cell
	}
	path = padRight(path, pathWidth)
	marker := changeMarker(file.Change)

	if selected {
		return view.th.Selected.Render("> " + viewedGlyph(file.Viewed) + " " + marker + " " + path + " " + counts)
	}
	mStyle := view.th.FileModified
	switch file.Change {
	case github.FileAdded:
		mStyle = view.th.FileAdded
	case github.FileDeleted:
		mStyle = view.th.FileDeleted
	case github.FileModified, github.FileRenamed, github.FileCopied, github.FileChanged:
	}
	var names []string
	if file.Additions > 0 {
		names = append(names, view.th.FileAdded.Render(fmt.Sprintf("+%d", file.Additions)))
	}
	if file.Deletions > 0 {
		names = append(names, view.th.FileDeleted.Render(fmt.Sprintf("-%d", file.Deletions)))
	}
	pathStyle, glyph := view.th.Normal, " "
	switch file.Viewed {
	case github.FileViewed:
		// Viewed files recede; the check is the only bright part of the row.
		pathStyle, glyph = view.th.Muted, view.th.FileAdded.Render(viewedGlyph(file.Viewed))
	case github.FileDismissed:
		// Changed since viewed: needs another look, so the row stays normal.
		glyph = view.th.FileModified.Render(viewedGlyph(file.Viewed))
	case github.FileUnviewed:
	}
	return "  " + glyph + " " + mStyle.Render(marker) + " " + pathStyle.Render(path) + " " + strings.Join(names, " ")
}

// viewedGlyph is the one-cell mark of a file's viewed state.
func viewedGlyph(state github.ViewedState) string {
	switch state {
	case github.FileViewed:
		return "✓"
	case github.FileDismissed:
		return "↻"
	case github.FileUnviewed:
	}
	return " "
}

func changeMarker(change github.FileChangeType) string {
	switch change {
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
func (view *View) rightRows(width, height int) []string {
	if view.selection() > 0 {
		return strings.Split(view.filePane.Render(width, height), "\n")
	}
	lines := strings.Split(view.ov.vp.View(), "\n")
	for i, line := range lines {
		lines[i] = " " + line
	}
	return lines
}

func padRight(text string, width int) string {
	if count := width - ansi.StringWidth(text); count > 0 {
		return text + strings.Repeat(" ", count)
	}
	return text
}

// drawPane frames lines in a titled rounded box of exactly w x h cells.
func drawPane(view *View, title string, lines []string, width, height int, focused bool) []string {
	border, titleStyle := view.th.PaneBorder, view.th.PaneTitle
	if focused {
		border, titleStyle = view.th.PaneBorderFocus, view.th.PaneTitleFocus
	}
	inner := width - 2
	title = ansi.Truncate(title, max(0, inner-3), "…")
	text := " " + title + " "
	top := border.Render("╭─") + titleStyle.Render(text) +
		border.Render(strings.Repeat("─", max(0, inner-1-ansi.StringWidth(text)))+"╮")
	out := make([]string, 0, height)
	out = append(out, top)
	side := border.Render("│")
	for i := range height - 2 {
		var line string
		if i < len(lines) {
			line = lines[i]
		}
		line = padRight(ansi.Truncate(line, inner, ""), inner)
		out = append(out, side+line+side)
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return out
}

// overlay draws box centred horizontally over rows, starting at row top.
func overlay(rows []string, box string, width, top int) {
	if box == "" {
		return
	}
	boxLines := strings.Split(box, "\n")
	boxWidth := ansi.StringWidth(boxLines[0])
	column := max(0, (width-boxWidth)/2)
	for i, line := range boxLines {
		row := top + 1 + i
		if row >= len(rows) {
			return
		}
		base := padRight(rows[row], width)
		rows[row] = ansi.Cut(base, 0, column) + line + ansi.Cut(base, column+boxWidth, width)
	}
}
