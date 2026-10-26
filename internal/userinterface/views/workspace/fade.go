package workspace

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MikePors/strigidae/internal/userinterface/theme"
)

// How long something marked for removal stays before it disappears, and the
// time between its fade steps: step 0 is the normal look, steps 1 to
// theme.FadeSteps the fading ones, and the step after that is gone.
const (
	fadeDuration = 8 * time.Second
	fadeInterval = fadeDuration / (theme.FadeSteps + 1)
)

// fadeMessage is a fade timer going off. It names the workspace, the thing
// fading and the run it belongs to, so a timer of a run that was cancelled or
// restarted is ignored.
type fadeMessage struct {
	owner      *View
	key        string
	generation int
}

// fadeRun is one thing fading out.
type fadeRun struct {
	generation int
	started    time.Time
	step       int // 0 until the first timer, then 1..theme.FadeSteps
}

// fader tracks things that fade out before they go, by a caller-chosen key.
// It holds no timers: the owner schedules fadeMessage with fadeCommand and
// calls advance when one arrives.
type fader struct {
	runs       map[string]*fadeRun
	generation int
}

// begin starts (or restarts) the fade of key and returns its generation.
func (faded *fader) begin(key string, now time.Time) int {
	if faded.runs == nil {
		faded.runs = map[string]*fadeRun{}
	}
	faded.generation++
	faded.runs[key] = &fadeRun{generation: faded.generation, started: now}
	return faded.generation
}

// cancel stops the fade of key; the thing stays as it was.
func (faded *fader) cancel(key string) { delete(faded.runs, key) }

// clear stops every fade.
func (faded *fader) clear() { clear(faded.runs) }

// active reports whether key is fading, including at its normal look.
func (faded *fader) active(key string) bool { return faded.runs[key] != nil }

// step returns how far key has faded: 0 is the normal look, theme.FadeSteps
// the faintest.
func (faded *fader) step(key string) int {
	if run := faded.runs[key]; run != nil {
		return run.step
	}
	return 0
}

// advance moves key's run to where now puts it, if generation is still the
// run's. It reports whether the message was current, and whether the run is
// over (and so removed).
func (faded *fader) advance(key string, generation int, now time.Time) (current, over bool) {
	run := faded.runs[key]
	if run == nil || run.generation != generation {
		return false, false
	}
	run.step = int(now.Sub(run.started) / fadeInterval)
	if run.step > theme.FadeSteps {
		delete(faded.runs, key)
		return true, true
	}
	return true, false
}

// fadeCommand asks for the next timer of key's run.
func (view *View) fadeCommand(key string, generation int) tea.Cmd {
	return view.tick(fadeInterval, func(time.Time) tea.Msg {
		return fadeMessage{owner: view, key: key, generation: generation}
	})
}
