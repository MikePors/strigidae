package workspace

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
	"github.com/MikePors/strigidae/internal/ui/components/thread"
	"github.com/MikePors/strigidae/internal/ui/termtext"
	"github.com/MikePors/strigidae/internal/ui/theme"
)

// overview is the right pane's content while Overview is selected: the
// description followed by the conversation, in a scrollable viewport.
type overview struct {
	vp       viewport.Model
	width    int       // content width the viewport was built for
	builtAt  time.Time // minute the content was built in; relative times age with it
	dirty    bool
	sections []int // first line of the description and of the conversation
}

func newOverview() overview {
	return overview{vp: viewport.New(), dirty: true}
}

// invalidate forces a rebuild before the next render.
func (o *overview) invalidate() { o.dirty = true }

// ensure sizes the viewport and rebuilds the content when the data, theme or
// width changed. Markdown rendering is cached by the renderer.
func (o *overview) ensure(th theme.Theme, md *markdown.Renderer, d *github.PullRequestDetail, threads threadsView, now time.Time, width, height int) {
	o.vp.SetWidth(width)
	o.vp.SetHeight(height)
	minute := now.Truncate(time.Minute)
	if d == nil || (!o.dirty && o.width == width && minute.Equal(o.builtAt)) {
		return
	}
	var b strings.Builder
	o.sections = o.sections[:0]
	line := 0
	add := func(s string) {
		b.WriteString(s)
		b.WriteString("\n")
		line += strings.Count(s, "\n") + 1
	}

	o.sections = append(o.sections, line)
	add(th.SectionHeading.Render("Description"))
	if strings.TrimSpace(d.Body) == "" {
		add(th.Muted.Render("No description provided."))
	} else {
		add(md.Render(termtext.CleanBlock(d.Body), width))
	}
	add("")

	o.sections = append(o.sections, line)
	items := conversation(d)
	add(th.SectionHeading.Render(fmt.Sprintf("Conversation (%d)", len(items))))
	if len(items) == 0 {
		add(th.Muted.Render("No comments yet."))
	}
	for _, it := range items {
		add(it.header(th, now))
		if strings.TrimSpace(it.body) != "" {
			add(md.Render(termtext.CleanBlock(it.body), width))
		}
		add("")
	}

	o.sections = append(o.sections, line)
	switch {
	case !threads.loaded && threads.pending:
		add(th.SectionHeading.Render("Review threads"))
		add(th.Muted.Render("Loading review threads…"))
	case !threads.loaded && threads.err != nil:
		add(th.SectionHeading.Render("Review threads"))
		add(th.ErrorText.Render(ansi.Wrap("Could not load review threads: "+termtext.SafeError(threads.err).Error(), width, "")))
	default:
		add(th.SectionHeading.Render(fmt.Sprintf("Review threads (%d)", len(threads.list))))
		if len(threads.list) == 0 {
			add(th.Muted.Render("No review threads."))
		}
	}
	env := thread.Env{Theme: th, Markdown: md, Now: now}
	for _, t := range threads.list {
		add(strings.Join(thread.Lines(env, t, width, thread.Options{Expanded: true, Location: true}), "\n"))
		add("")
	}

	o.vp.SetContent(strings.TrimRight(b.String(), "\n"))
	o.width, o.builtAt, o.dirty = width, minute, false
}

// jump scrolls to the next (delta > 0) or previous section start.
func (o *overview) jump(delta int) {
	y := o.vp.YOffset()
	if delta > 0 {
		for _, s := range o.sections {
			if s > y {
				o.vp.SetYOffset(s)
				return
			}
		}
		return
	}
	for _, s := range slices.Backward(o.sections) {
		if s < y {
			o.vp.SetYOffset(s)
			return
		}
	}
}

// threadsView is the review threads and how loading them stands, as the
// Overview needs them.
type threadsView struct {
	list    []github.ReviewThread
	loaded  bool // a load has succeeded
	pending bool // a load is in flight
	err     error
}

// entry is one item of the conversation: a top-level comment or a review.
type entry struct {
	author  string
	at      time.Time
	action  string
	body    string
	pending bool
}

func (e entry) header(th theme.Theme, now time.Time) string {
	if e.pending {
		return th.Author.Render(termtext.Clean(e.author)) + " " + th.Pending.Render("PENDING "+e.action+" (draft, not submitted)")
	}
	return th.Author.Render(termtext.Clean(e.author)) + " " + e.action + th.Muted.Render(" · "+thread.RelTime(now, e.at))
}

// conversation merges comments and review summaries chronologically. Pending
// reviews have no submission time and come last. Reviews without a body that
// only wrap inline comments ("commented") are left out; verdicts are kept.
func conversation(d *github.PullRequestDetail) []entry {
	var out []entry
	for _, c := range d.Comments {
		out = append(out, entry{author: c.Author, at: c.CreatedAt, action: "commented", body: c.Body})
	}
	for _, r := range d.Reviews {
		e := entry{author: r.Author, at: r.SubmittedAt, action: reviewAction(r.State), body: r.Body}
		switch {
		case r.State == github.ReviewPending || r.SubmittedAt.IsZero():
			e.pending, e.action = true, "review"
		case r.State == github.ReviewCommented && strings.TrimSpace(r.Body) == "":
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b entry) int {
		if a.pending != b.pending {
			if a.pending {
				return 1
			}
			return -1
		}
		return a.at.Compare(b.at)
	})
	return out
}

func reviewAction(s github.ReviewState) string {
	switch s {
	case github.ReviewApproved:
		return "approved"
	case github.ReviewChangesRequested:
		return "requested changes"
	case github.ReviewDismissed:
		return "review dismissed"
	default:
		return "reviewed"
	}
}

// sortThreads orders threads by file, then line, keeping the server's order
// for threads on the same line. It returns a copy.
func sortThreads(ts []github.ReviewThread) []github.ReviewThread {
	out := slices.Clone(ts)
	line := func(t github.ReviewThread) int {
		if t.Line > 0 {
			return t.Line
		}
		return t.OriginalLine
	}
	slices.SortStableFunc(out, func(a, b github.ReviewThread) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return line(a) - line(b)
	})
	return out
}
