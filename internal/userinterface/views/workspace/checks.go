package workspace

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/userinterface"
	"github.com/MikePors/strigidae/internal/userinterface/components/picker"
	"github.com/MikePors/strigidae/internal/userinterface/terminaltext"
	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// sortedChecks orders checks failing first, then running or queued, then
// passing, then skipped or neutral, alphabetically within each group. It
// returns a copy.
func sortedChecks(checks []github.Check) []github.Check {
	out := slices.Clone(checks)
	slices.SortStableFunc(out, func(first, second github.Check) int {
		if group := int(first.Result.Group()) - int(second.Result.Group()); group != 0 {
			return group
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return out
}

// checkGlyph returns the one-cell mark of a check and its style. The colours
// are the pull request list's checks styles; skipped and neutral are muted.
func checkGlyph(activeTheme theme.Theme, result github.CheckResult) (string, lipgloss.Style) {
	styles := activeTheme.PullRequestList
	switch result.Group() {
	case github.CheckGroupFailing:
		return "✗", styles.ChecksFailing
	case github.CheckGroupPending:
		if result == github.CheckQueued {
			return "○", styles.ChecksRunning
		}
		return "●", styles.ChecksRunning
	case github.CheckGroupPassing:
		return "✓", styles.ChecksPassing
	case github.CheckGroupInactive:
	}
	return "-", activeTheme.Muted
}

// checkLabel is the plain text of a check row: glyph, name and state in
// words. The picker lists it as is.
func checkLabel(activeTheme theme.Theme, check github.Check) string {
	glyph, _ := checkGlyph(activeTheme, check.Result)
	return glyph + " " + terminaltext.Clean(check.Name) + "  " + check.Result.Word()
}

// checkRow is a styled Overview row that fits width; a long name is cut so
// the state stays visible.
func checkRow(activeTheme theme.Theme, check github.Check, width int) string {
	glyph, style := checkGlyph(activeTheme, check.Result)
	word := check.Result.Word()
	name := terminaltext.Clean(check.Name)
	if room := width - 2 - 2 - ansi.StringWidth(word); ansi.StringWidth(name) > room {
		name = ansi.Truncate(name, max(room, 1), "…")
	}
	return style.Render(glyph) + " " + activeTheme.Normal.Render(name) + "  " + style.Render(word)
}

// checkSectionLines are the Overview's Checks section: heading and rows.
func checkSectionLines(activeTheme theme.Theme, detail *github.PullRequestDetail, width int) []string {
	total := max(detail.CheckTotal, len(detail.CheckList))
	lines := []string{activeTheme.SectionHeading.Render(fmt.Sprintf("Checks (%d)", total))}
	if total == 0 {
		return append(lines, activeTheme.Muted.Render("No checks."))
	}
	for _, check := range sortedChecks(detail.CheckList) {
		lines = append(lines, checkRow(activeTheme, check, width))
	}
	if more := total - len(detail.CheckList); more > 0 {
		lines = append(lines, activeTheme.Muted.Render(fmt.Sprintf("and %d more", more)))
	}
	return lines
}

// checksCommand handles :checks: it opens a picker of the same rows.
func (view *View) checksCommand() tea.Cmd {
	switch {
	case view.detail == nil:
		return userinterface.Info("pull request not loaded yet")
	case len(view.detail.CheckList) == 0:
		return userinterface.Info("No checks.")
	case view.picker != nil || view.review.chooser != nil:
		return nil
	}
	checks := sortedChecks(view.detail.CheckList)
	items := make([]string, len(checks))
	for i, check := range checks {
		items[i] = checkLabel(view.theme, check)
	}
	view.pickerChecks = checks
	view.picker = picker.New(view.theme, "Checks", items)
	if more := max(view.detail.CheckTotal, len(checks)) - len(checks); more > 0 {
		view.picker.SetNote(fmt.Sprintf("and %d more", more))
	}
	return nil
}

// openCheck opens a check's details page in the browser, the way `o` opens
// the pull request. Only http and https links are opened: a commit status's
// target URL is set by whoever posts the status.
func (view *View) openCheck(check github.Check) tea.Cmd {
	address, err := url.Parse(check.URL)
	if check.URL == "" || err != nil || (address.Scheme != "https" && address.Scheme != "http") {
		return userinterface.Info("this check has no link to open")
	}
	return view.openAddress(check.URL)
}
