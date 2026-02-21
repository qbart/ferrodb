package ui

import "github.com/charmbracelet/lipgloss"

type Sidebar struct {
	Title   string
	Version string
	Tree    Tree
	theme   Theme
}

func NewSidebar(theme Theme) Sidebar {
	return Sidebar{Title: "Database", Tree: NewTree(theme), theme: theme}
}

func (s Sidebar) View(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(false).
		Background(s.theme.SidebarHeaderBg).
		Foreground(s.theme.SidebarHeaderFg)

	versionStyle := lipgloss.NewStyle().
		Background(s.theme.SidebarHeaderBg).
		Foreground(s.theme.Muted).
		Bold(false)

	headerStyle := lipgloss.NewStyle().
		Background(s.theme.SidebarHeaderBg).
		Width(width).
		Bold(false)

	spaceStyle := lipgloss.NewStyle().Background(s.theme.SidebarHeaderBg).Bold(false)

	content := " " + titleStyle.Render(s.Title)
	if s.Version != "" {
		content += spaceStyle.Render(" ") + versionStyle.Render(s.Version)
	}
	header := headerStyle.Render(content)

	bodyHeight := max(0, height-1)
	bodyStyle := lipgloss.NewStyle().
		Background(s.theme.SidebarBg).
		Foreground(s.theme.Fg).
		Width(width).
		Height(bodyHeight).
		Bold(false).
		Padding(1)

	innerWidth := max(0, width-2)
	innerHeight := max(0, bodyHeight-2)
	body := bodyStyle.Render(s.Tree.View(innerWidth, innerHeight))

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
