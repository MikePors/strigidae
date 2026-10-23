package theme

import "charm.land/lipgloss/v2"

// DashboardStyles are the styles for the dashboard's tab bar.
type DashboardStyles struct {
	ActiveTab   lipgloss.Style // the tab being shown
	InactiveTab lipgloss.Style // every other tab
}

func newDashboardStyles(roles Roles) DashboardStyles {
	return DashboardStyles{
		ActiveTab:   lipgloss.NewStyle().Bold(true).Foreground(roles.OnAccent).Background(roles.Accent),
		InactiveTab: lipgloss.NewStyle().Foreground(roles.Muted),
	}
}
