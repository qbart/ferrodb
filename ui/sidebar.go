package ui

import "github.com/charmbracelet/lipgloss"

type Sidebar struct {
	Title string
	Tree  Tree
	theme Theme
}

func NewSidebar(theme Theme) Sidebar {
	return Sidebar{Title: "Database", Tree: NewTree(theme), theme: theme}
}

func (s Sidebar) View(width, height int) string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Background(s.theme.SidebarHeaderBg).
		Foreground(s.theme.SidebarHeaderFg).
		Width(width)

	header := headerStyle.Render(" " + s.Title)

	bodyHeight := max(0, height-1)
	bodyStyle := lipgloss.NewStyle().
		Background(s.theme.SidebarBg).
		Foreground(s.theme.Fg).
		Width(width).
		Height(bodyHeight).
		Padding(1)

	innerWidth := max(0, width-2)
	innerHeight := max(0, bodyHeight-2)
	body := bodyStyle.Render(s.Tree.View(innerWidth, innerHeight))

	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
