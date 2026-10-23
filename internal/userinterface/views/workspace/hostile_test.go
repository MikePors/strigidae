package workspace

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/diff"
	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/userinterface"
)

// hostile is attacker-controlled text: CSI and OSC sequences (title change,
// OSC 8 hyperlink, OSC 52 clipboard write), C1 controls, bidi overrides and
// isolates, a bare carriage return, and invalid UTF-8.
const hostile = "evil\x1b[2J\x1b]0;pwned\x07\x1b]8;;http://evil.test\x07link\x1b]8;;\x07" +
	"\x1b]52;c;aGk=\x07\u009b31m\u0090dcs\u009c\u202eevil\u2066x\u2069\rafter\xff\xfe\x00end"

// references are HTML character references that Glamour decodes into real escape
// sequences and bidi controls unless its output is cleaned.
const references = "&#x1b;[2J &#x1b;]52;c;aGk=&#x07; &#x202e;gnp.exe &rlm; &lrm; &#27;[2J &#7;"

// referencesBody exercises the references in text, inline HTML, a code span, a
// fenced block, link text and a heading.
const referencesBody = "\n\n" + references + "\n\n<span>&#x1b;]0;t&#x07;</span>\n\n`&#x1b;]52;c;aGk=&#x07;`\n\n" +
	"```\n&#x1b;[2J &#x202e;\n```\n\n[&#x1b;[2J](http://x/)\n\n# &#x1b;]0;h&#x07;"

var (
	hostileGoPath  = "dir/" + hostile + ".go"
	hostileOldPath = "old/" + hostile + ".md"
	hostileNewPath = "ren/" + hostile + ".md"
)

// hostileDiff is a diff whose paths, mode lines, section text and code lines
// all carry hostile text, plus a rename with hostile old and new names.
func hostileDiff() string {
	return "diff --git a/" + hostileGoPath + " b/" + hostileGoPath + "\n" +
		"old mode 100644\nnew mode 100755\n" +
		"--- a/" + hostileGoPath + "\n+++ b/" + hostileGoPath + "\n" +
		"@@ -1,1 +1,1 @@ section " + hostile + "\n" +
		"-old " + hostile + references + "\n+new " + hostile + references + "\n" +
		"diff --git a/" + hostileOldPath + " b/" + hostileNewPath + "\n" +
		"similarity index 100%\nrename from " + hostileOldPath + "\nrename to " + hostileNewPath + "\n"
}

func hostileDetail() github.PullRequestDetail {
	detail := testDetail()
	detail.Title = "title " + hostile
	detail.Author = "author" + hostile
	detail.HeadBranch = "head" + hostile
	detail.BaseBranch = "base" + hostile
	detail.Body = "# body " + hostile + "\n\n[click](http://x)\x1b[31m red\r\n" + hostile + referencesBody
	detail.Labels = []github.Label{{Name: "label" + hostile}}
	detail.ReviewRequests = []github.Reviewer{{Name: "rev" + hostile}, {Name: "team" + hostile, IsTeam: true}}
	detail.Files = append(detail.Files,
		github.ChangedFile{Path: hostileGoPath, Additions: 1, Deletions: 1, Change: github.FileModified},
		github.ChangedFile{Path: hostileNewPath, Change: github.FileRenamed})
	detail.Comments = []github.IssueComment{{Author: "c" + hostile, CreatedAt: testNow.Add(-time.Hour), Body: hostile + referencesBody}}
	detail.Reviews = []github.Review{
		{Author: "r" + hostile, State: github.ReviewApproved, SubmittedAt: testNow.Add(-time.Hour), Body: hostile + referencesBody},
		{Author: "p" + hostile, State: github.ReviewPending, Body: hostile + referencesBody},
	}
	return detail
}

// hostileThreads are review threads whose path, authors and bodies carry
// hostile text, character references included: one on the hostile file's new
// line 1, one outdated, one resolved on the deleted line, and one on another file.
func hostileThreads() []github.ReviewThread {
	comment := func(who string) github.ReviewComment {
		return github.ReviewComment{
			Identifier: who, Author: who + hostile, Body: hostile + referencesBody, CreatedAt: testNow.Add(-time.Hour),
			Reactions: []github.ReactionSummary{{Content: "X" + github.ReactionContent(hostile), Count: 2}},
		}
	}
	return []github.ReviewThread{
		{Identifier: "h1", Path: hostileGoPath, Line: 1, DiffSide: github.SideRight, Comments: []github.ReviewComment{comment("a"), comment("b")}},
		{Identifier: "h2", Path: hostileGoPath, OriginalLine: 1, IsOutdated: true, Comments: []github.ReviewComment{comment("c")}},
		{Identifier: "h3", Path: hostileGoPath, Line: 1, DiffSide: github.SideLeft, IsResolved: true, Comments: []github.ReviewComment{comment("d")}},
		{Identifier: "h4", Path: "elsewhere/" + hostile, Line: 3, Comments: []github.ReviewComment{comment("e")}},
	}
}

// styleOnly matches the SGR sequences our own styles emit.
var styleOnly = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// osc8 matches the well-formed hyperlinks Glamour itself emits for markdown
// links; the URL is attacker text, but without ESC and BEL it cannot end the
// sequence early.
var osc8 = regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;([^\x07\x1b]*)\x07`)

// assertTame fails if out contains anything but plain text and SGR styling.
func assertTame(test *testing.T, what, out string) {
	test.Helper()
	for _, messages := range osc8.FindAllStringSubmatch(out, -1) {
		if strings.Contains(messages[1], "evil") {
			test.Errorf("%s: hostile text became a hyperlink target: %q", what, messages[1])
		}
	}
	out = osc8.ReplaceAllString(out, "")
	if rest := styleOnly.ReplaceAllString(out, ""); strings.ContainsRune(rest, 0x1b) {
		test.Errorf("%s: output holds a non-SGR escape: %q", what, rest)
	}
	for _, character := range ansi.Strip(out) {
		bad := character == '\r' || character == 0x7f || (unicode.IsControl(character) && character != '\n' && character != '\t') ||
			(character >= 0x200b && character <= 0x200f) || (character >= 0x202a && character <= 0x202e) || (character >= 0x2066 && character <= 0x2069)
		if bad {
			test.Errorf("%s: output holds control or bidi rune U+%04X", what, character)
		}
	}
}

func TestHostileTextNeverReachesTheScreen(test *testing.T) {
	client := fake.New()
	detail := hostileDetail()
	client.SetPullRequest(detail)
	client.SetDiff(testRepository, 7, hostileDiff())
	client.SetReviewThreads(testRepository, 7, hostileThreads())
	if parsed, err := diff.Parse(hostileDiff()); err != nil || parsed.File(hostileGoPath) == nil || parsed.File(hostileNewPath) == nil {
		test.Fatalf("setup: the hostile diff must parse so its content reaches the pane (err=%v)", err)
	}
	harness := newHarness(test, 140, 40)
	harness.client = client
	harness.view.client = client
	harness.run(harness.view.Init())

	assertTame(test, "overview", harness.render())
	harness.press("l", "G")
	assertTame(test, "overview conversation", harness.render())

	harness.press("h", "G") // the renamed hostile file is last
	assertTame(test, "renamed file selected", harness.render())
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "Renamed:") {
		test.Errorf("the rename line should reach the pane:\n%s", out)
	}
	harness.press("k") // the hostile modified file, with hostile hunk content
	assertTame(test, "file selected", harness.render())
	if !strings.Contains(harness.view.rightTitle(), "dir/evil") {
		test.Errorf("hostile path was dropped, not cleaned: %q", harness.view.rightTitle())
	}
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "old evil") || !strings.Contains(out, "new evil") {
		test.Errorf("hostile code lines should show, cleaned:\n%s", out)
	}
	harness.press("l", ")", "z o", ")", "z o", "G") // threads: open the resolved one, go to the end
	assertTame(test, "hostile threads", harness.render())
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "│ cevil") {
		test.Errorf("the hostile threads should show, cleaned:\n%s", out)
	}
	harness.press("h", "g g", "l", "G")
	assertTame(test, "overview with hostile threads", harness.render())
	if out := ansi.Strip(harness.render()); !strings.Contains(out, "│ eevil") {
		test.Errorf("the overview should list the threads:\n%s", out)
	}
	harness.press("h", "G", "k")
	harness.press("j", "j", "j") // the hostile diff lines under the cursor, too
	assertTame(test, "cursor in hostile hunk", harness.render())

	harness.press("f")
	harness.typeText("evil")
	assertTame(test, "picker", harness.render())
	harness.send(keyEsc())

	// Narrow terminal, then the picker open over it.
	harness.send(sizeMessage(60, 20))
	assertTame(test, "narrow", harness.render())
	harness.press("f")
	harness.typeText("evil")
	if !harness.view.CapturesKeys() {
		test.Fatal("setup: picker should be open")
	}
	out := harness.render()
	assertTame(test, "narrow with picker", out)
	if !strings.Contains(ansi.Strip(out), "Go to file") {
		test.Errorf("the picker should be drawn over the narrow layout:\n%s", ansi.Strip(out))
	}
}

func TestHostileErrorsAreCleaned(test *testing.T) {
	hostileErr := errors.New("boom" + hostile)
	harness := newHarness(test, 100, 20)
	harness.client.SetError(fake.MethodGetPullRequest, hostileErr)
	harness.run(harness.view.Init())
	assertTame(test, "load failure", harness.render())

	var got string
	for _, message := range harness.messages {
		if errorMessage, ok := message.(userinterface.ErrorMessage); ok {
			got = errorMessage.Err.Error()
		}
	}
	if got == "" {
		test.Fatal("no status bar error")
	}
	assertTame(test, "status bar error", got)

	// Opening the browser can fail with attacker text in the URL.
	secondHarness := loaded(test, 100, 20)
	secondHarness.view.detail.URL = "http://x/" + hostile
	secondHarness.view.open = func(string) error { return hostileErr }
	secondHarness.messages = nil
	secondHarness.press("o")
	for _, message := range secondHarness.messages {
		if errorMessage, ok := message.(userinterface.ErrorMessage); ok {
			assertTame(test, "open error", errorMessage.Err.Error())
		}
	}
}

func keyEsc() tea.Msg { return tea.KeyPressMsg{Code: tea.KeyEscape} }

func sizeMessage(width, height int) tea.Msg { return tea.WindowSizeMsg{Width: width, Height: height} }

func TestHostileDiffError(test *testing.T) {
	harness := newHarness(test, 100, 20)
	harness.client.SetError(fake.MethodGetDiff, errors.New("bad"+hostile))
	harness.run(harness.view.Init())
	harness.press("}")
	assertTame(test, "diff error", harness.render())

	var status string
	for _, message := range harness.messages {
		if errorMessage, ok := message.(userinterface.ErrorMessage); ok {
			status = errorMessage.Err.Error()
		}
	}
	if !strings.Contains(status, "bad") {
		test.Fatalf("the diff failure should reach the status bar, got %q", status)
	}
	assertTame(test, "diff error on the status bar", status)
}
