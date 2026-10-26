package theme

import "testing"

func TestFadeStylesApproachTheBackground(test *testing.T) {
	for _, dark := range []bool{true, false} {
		activeTheme := New(dark)
		previous := contrast(activeTheme.Roles.Muted, activeTheme.Roles.Background)
		for step, style := range activeTheme.Fade {
			current := contrast(style.GetForeground(), activeTheme.Roles.Background)
			if current >= previous {
				test.Errorf("dark=%v: fade step %d contrasts %.2f with the background, not less than the step before (%.2f)", dark, step+1, current, previous)
			}
			previous = current
		}
	}
}
