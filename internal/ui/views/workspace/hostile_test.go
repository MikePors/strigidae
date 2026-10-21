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

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/github/fake"
	"github.com/MikePors/strigidae/internal/ui"
)

// hostile is attacker-controlled text: CSI and OSC sequences (title change,
// OSC 8 hyperlink, OSC 52 clipboard write), C1 controls, bidi overrides and
// isolates, a bare carriage return, and invalid UTF-8.
const hostile = "evil\x1b[2J\x1b]0;pwned\x07\x1b]8;;http://evil.test\x07link\x1b]8;;\x07" +
	"\x1b]52;c;aGk=\x07\u009b31m\u0090dcs\u009c\u202eevil\u2066x\u2069\rafter\xff\xfe\x00end"

func hostileDetail() github.PullRequestDetail {
	d := testDetail()
	d.Title = "title " + hostile
	d.Author = "author" + hostile
	d.HeadRef = "head" + hostile
	d.BaseRef = "base" + hostile
	d.Body = "# body " + hostile + "\n\n[click](http://x)\x1b[31m red\r\n" + hostile
	d.Labels = []github.Label{{Name: "label" + hostile}}
	d.ReviewRequests = []github.Reviewer{{Name: "rev" + hostile}, {Name: "team" + hostile, IsTeam: true}}
	d.Files = append(d.Files, github.ChangedFile{Path: "dir/" + hostile + ".go", Additions: 1, Change: github.FileAdded})
	d.Comments = []github.IssueComment{{Author: "c" + hostile, CreatedAt: testNow.Add(-time.Hour), Body: hostile}}
	d.Reviews = []github.Review{
		{Author: "r" + hostile, State: github.ReviewApproved, SubmittedAt: testNow.Add(-time.Hour), Body: hostile},
		{Author: "p" + hostile, State: github.ReviewPending, Body: hostile},
	}
	return d
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
	c.SetDiff(testRepo, 7, readDiff(t))
	h := newHarness(t, 140, 40)
	h.client = c
	h.v.client = c
	h.run(h.v.Init())

	assertTame(t, "overview", h.render())
	h.press("l", "G")
	assertTame(t, "overview conversation", h.render())

	h.press("h", "G") // the hostile file name is last
	assertTame(t, "file selected", h.render())
	if !strings.Contains(h.v.rightTitle(), "dir/evil") {
		t.Errorf("hostile path was dropped, not cleaned: %q", h.v.rightTitle())
	}

	h.press("f")
	h.typ("evil")
	assertTame(t, "picker", h.render())
	h.send(keyEsc())

	// Narrow terminal and the picker over it.
	h.send(sizeMsg(60, 20))
	assertTame(t, "narrow", h.render())
}

func TestHostileErrorsAreCleaned(t *testing.T) {
	hostileErr := errors.New("boom" + hostile)
	h := newHarness(t, 100, 20)
	h.client.SetError(fake.MethodGetPullRequest, hostileErr)
	h.run(h.v.Init())
	assertTame(t, "load failure", h.render())

	var got string
	for _, m := range h.msgs {
		if e, ok := m.(ui.ErrorMsg); ok {
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
		if e, ok := m.(ui.ErrorMsg); ok {
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
	h.press("] f")
	assertTame(t, "diff error", h.render())
}
