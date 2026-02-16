package ui

import "github.com/charmbracelet/lipgloss"

type Content struct {
	theme Theme
}

func NewContent(theme Theme) Content {
	return Content{theme: theme}
}

func (c Content) View(width, height int) string {
	style := lipgloss.NewStyle().
		Background(c.theme.Bg).
		Foreground(c.theme.Fg).
		Width(width).
		Height(height)

	return style.Render(lipgloss.NewStyle().
		Foreground(c.theme.Muted).
		Background(c.theme.Bg).
		Render(" Content"))
}
