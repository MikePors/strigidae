package theme

import "charm.land/lipgloss/v2"

// FadeSteps is how many faded styles Theme.Fade holds.
const FadeSteps = 4

// fadeStyles blends the muted role toward the background in even steps, the
// last one still faintly visible. Muted is where a viewed file's row starts.
func fadeStyles(roles Roles) [FadeSteps]lipgloss.Style {
	var steps [FadeSteps]lipgloss.Style
	for i := range steps {
		fraction := float64(i+1) / float64(FadeSteps+1)
		steps[i] = lipgloss.NewStyle().Foreground(blend(roles.Muted, roles.Background, fraction))
	}
	return steps
}
