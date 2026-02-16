package ui

import "github.com/charmbracelet/lipgloss"

type Sidebar struct {
	theme Theme
}

func NewSidebar(theme Theme) Sidebar {
	return Sidebar{theme: theme}
}

func (s Sidebar) View(width, height int) string {
	style := lipgloss.NewStyle().
		Background(s.theme.SidebarBg).
		Foreground(s.theme.Fg).
		Width(width).
		Height(height)

	return style.Render(lipgloss.NewStyle().
		Foreground(s.theme.Muted).
		Background(s.theme.SidebarBg).
		Render(" Tree"))
}
