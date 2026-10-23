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

// refs are HTML character references that Glamour decodes into real escape
// sequences and bidi controls unless its output is cleaned.
const refs = "&#x1b;[2J &#x1b;]52;c;aGk=&#x07; &#x202e;gnp.exe &rlm; &lrm; &#27;[2J &#7;"

// refsBody exercises the references in text, inline HTML, a code span, a
// fenced block, link text and a heading.
const refsBody = "\n\n" + refs + "\n\n<span>&#x1b;]0;t&#x07;</span>\n\n`&#x1b;]52;c;aGk=&#x07;`\n\n" +
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
		"-old " + hostile + refs + "\n+new " + hostile + refs + "\n" +
		"diff --git a/" + hostileOldPath + " b/" + hostileNewPath + "\n" +
		"similarity index 100%\nrename from " + hostileOldPath + "\nrename to " + hostileNewPath + "\n"
}

func hostileDetail() github.PullRequestDetail {
	d := testDetail()
	d.Title = "title " + hostile
	d.Author = "author" + hostile
	d.HeadBranch = "head" + hostile
	d.BaseBranch = "base" + hostile
	d.Body = "# body " + hostile + "\n\n[click](http://x)\x1b[31m red\r\n" + hostile + refsBody
	d.Labels = []github.Label{{Name: "label" + hostile}}
	d.ReviewRequests = []github.Reviewer{{Name: "rev" + hostile}, {Name: "team" + hostile, IsTeam: true}}
	d.Files = append(d.Files,
		github.ChangedFile{Path: hostileGoPath, Additions: 1, Deletions: 1, Change: github.FileModified},
		github.ChangedFile{Path: hostileNewPath, Change: github.FileRenamed})
	d.Comments = []github.IssueComment{{Author: "c" + hostile, CreatedAt: testNow.Add(-time.Hour), Body: hostile + refsBody}}
	d.Reviews = []github.Review{
		{Author: "r" + hostile, State: github.ReviewApproved, SubmittedAt: testNow.Add(-time.Hour), Body: hostile + refsBody},
		{Author: "p" + hostile, State: github.ReviewPending, Body: hostile + refsBody},
	}
	return d
}

// hostileThreads are review threads whose path, authors and bodies carry
// hostile text, character references included: one on the hostile file's new
// line 1, one outdated, one resolved on the deleted line, and one on another file.
func hostileThreads() []github.ReviewThread {
	cmt := func(who string) github.ReviewComment {
		return github.ReviewComment{
			Identifier: who, Author: who + hostile, Body: hostile + refsBody, CreatedAt: testNow.Add(-time.Hour),
			Reactions: []github.ReactionSummary{{Content: "X" + github.ReactionContent(hostile), Count: 2}},
		}
	}
	return []github.ReviewThread{
		{Identifier: "h1", Path: hostileGoPath, Line: 1, DiffSide: github.SideRight, Comments: []github.ReviewComment{cmt("a"), cmt("b")}},
		{Identifier: "h2", Path: hostileGoPath, OriginalLine: 1, IsOutdated: true, Comments: []github.ReviewComment{cmt("c")}},
		{Identifier: "h3", Path: hostileGoPath, Line: 1, DiffSide: github.SideLeft, IsResolved: true, Comments: []github.ReviewComment{cmt("d")}},
		{Identifier: "h4", Path: "elsewhere/" + hostile, Line: 3, Comments: []github.ReviewComment{cmt("e")}},
	}
}

// styleOnly matches the SGR sequences our own styles emit.
var styleOnly = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// osc8 matches the well-formed hyperlinks Glamour itself emits for markdown
// links; the URL is attacker text, but without ESC and BEL it cannot end the
// sequence early.
var osc8 = regexp.MustCompile(`\x1b\]8;[^;\x07\x1b]*;([^\x07\x1b]*)\x07`)

// assertTame fails if out contains anything but plain text and SGR styling.
func assertTame(t *testing.T, what, out string) {
	t.Helper()
	for _, m := range osc8.FindAllStringSubmatch(out, -1) {
		if strings.Contains(m[1], "evil") {
			t.Errorf("%s: hostile text became a hyperlink target: %q", what, m[1])
		}
	}
	out = osc8.ReplaceAllString(out, "")
	if rest := styleOnly.ReplaceAllString(out, ""); strings.ContainsRune(rest, 0x1b) {
		t.Errorf("%s: output holds a non-SGR escape: %q", what, rest)
	}
	for _, r := range ansi.Strip(out) {
		bad := r == '\r' || r == 0x7f || (unicode.IsControl(r) && r != '\n' && r != '\t') ||
			(r >= 0x200b && r <= 0x200f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
		if bad {
			t.Errorf("%s: output holds control or bidi rune U+%04X", what, r)
		}
	}
}

func TestHostileTextNeverReachesTheScreen(t *testing.T) {
	c := fake.New()
	d := hostileDetail()
	c.SetPullRequest(d)
	c.SetDiff(testRepo, 7, hostileDiff())
	c.SetReviewThreads(testRepo, 7, hostileThreads())
	if parsed, err := diff.Parse(hostileDiff()); err != nil || parsed.File(hostileGoPath) == nil || parsed.File(hostileNewPath) == nil {
		t.Fatalf("setup: the hostile diff must parse so its content reaches the pane (err=%v)", err)
	}
	h := newHarness(t, 140, 40)
	h.client = c
	h.v.client = c
	h.run(h.v.Init())

	assertTame(t, "overview", h.render())
	h.press("l", "G")
	assertTame(t, "overview conversation", h.render())

	h.press("h", "G") // the renamed hostile file is last
	assertTame(t, "renamed file selected", h.render())
	if out := ansi.Strip(h.render()); !strings.Contains(out, "Renamed:") {
		t.Errorf("the rename line should reach the pane:\n%s", out)
	}
	h.press("k") // the hostile modified file, with hostile hunk content
	assertTame(t, "file selected", h.render())
	if !strings.Contains(h.v.rightTitle(), "dir/evil") {
		t.Errorf("hostile path was dropped, not cleaned: %q", h.v.rightTitle())
	}
	if out := ansi.Strip(h.render()); !strings.Contains(out, "old evil") || !strings.Contains(out, "new evil") {
		t.Errorf("hostile code lines should show, cleaned:\n%s", out)
	}
	h.press("l", ")", "z o", ")", "z o", "G") // threads: open the resolved one, go to the end
	assertTame(t, "hostile threads", h.render())
	if out := ansi.Strip(h.render()); !strings.Contains(out, "│ cevil") {
		t.Errorf("the hostile threads should show, cleaned:\n%s", out)
	}
	h.press("h", "g g", "l", "G")
	assertTame(t, "overview with hostile threads", h.render())
	if out := ansi.Strip(h.render()); !strings.Contains(out, "│ eevil") {
		t.Errorf("the overview should list the threads:\n%s", out)
	}
	h.press("h", "G", "k")
	h.press("j", "j", "j") // the hostile diff lines under the cursor, too
	assertTame(t, "cursor in hostile hunk", h.render())

	h.press("f")
	h.typ("evil")
	assertTame(t, "picker", h.render())
	h.send(keyEsc())

	// Narrow terminal, then the picker open over it.
	h.send(sizeMsg(60, 20))
	assertTame(t, "narrow", h.render())
	h.press("f")
	h.typ("evil")
	if !h.v.CapturesKeys() {
		t.Fatal("setup: picker should be open")
	}
	out := h.render()
	assertTame(t, "narrow with picker", out)
	if !strings.Contains(ansi.Strip(out), "Go to file") {
		t.Errorf("the picker should be drawn over the narrow layout:\n%s", ansi.Strip(out))
	}
}

func TestHostileErrorsAreCleaned(t *testing.T) {
	hostileErr := errors.New("boom" + hostile)
	h := newHarness(t, 100, 20)
	h.client.SetError(fake.MethodGetPullRequest, hostileErr)
	h.run(h.v.Init())
	assertTame(t, "load failure", h.render())

	var got string
	for _, m := range h.msgs {
		if e, ok := m.(userinterface.ErrorMsg); ok {
			got = e.Err.Error()
		}
	}
	if got == "" {
		t.Fatal("no status bar error")
	}
	assertTame(t, "status bar error", got)

	// Opening the browser can fail with attacker text in the URL.
	h2 := loaded(t, 100, 20)
	h2.v.detail.URL = "http://x/" + hostile
	h2.v.open = func(string) error { return hostileErr }
	h2.msgs = nil
	h2.press("o")
	for _, m := range h2.msgs {
		if e, ok := m.(userinterface.ErrorMsg); ok {
			assertTame(t, "open error", e.Err.Error())
		}
	}
}

func keyEsc() tea.Msg { return tea.KeyPressMsg{Code: tea.KeyEscape} }

func sizeMsg(w, h int) tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }

func TestHostileDiffError(t *testing.T) {
	h := newHarness(t, 100, 20)
	h.client.SetError(fake.MethodGetDiff, errors.New("bad"+hostile))
	h.run(h.v.Init())
	h.press("}")
	assertTame(t, "diff error", h.render())

	var status string
	for _, m := range h.msgs {
		if e, ok := m.(userinterface.ErrorMsg); ok {
			status = e.Err.Error()
		}
	}
	if !strings.Contains(status, "bad") {
		t.Fatalf("the diff failure should reach the status bar, got %q", status)
	}
	assertTame(t, "diff error on the status bar", status)
}
