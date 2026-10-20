package workspace

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"

	"github.com/MikePors/strigidae/internal/github"
	"github.com/MikePors/strigidae/internal/ui/components/markdown"
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
func (o *overview) ensure(th theme.Theme, md *markdown.Renderer, d *github.PullRequestDetail, now time.Time, width, height int) {
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
		add(md.Render(d.Body, width))
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
			add(md.Render(it.body, width))
		}
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
		return th.Author.Render(e.author) + " " + th.Pending.Render("PENDING "+e.action+" (draft, not submitted)")
	}
	return th.Author.Render(e.author) + " " + e.action + th.Muted.Render(" · "+relTime(now, e.at))
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

// relTime formats t relative to now ("5m ago").
func relTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy ago", int(d/(365*24*time.Hour)))
	}
}
