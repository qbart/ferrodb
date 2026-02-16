package ui

import "github.com/charmbracelet/lipgloss"

type NavItem int

const (
	NavDatabase NavItem = iota
	NavFavourites
)

var navIcons = map[NavItem]string{
	NavDatabase:   "\u26C1",
	NavFavourites: "\u2605",
}

var navItems = []NavItem{NavDatabase, NavFavourites}

const navWidth = 3

type Navbar struct {
	Active NavItem
	theme  Theme
}

func NewNavbar(theme Theme) Navbar {
	return Navbar{
		Active: NavDatabase,
		theme:  theme,
	}
}

func (n Navbar) View(height int) string {
	baseStyle := lipgloss.NewStyle().
		Background(n.theme.NavBg).
		Width(navWidth).
		Align(lipgloss.Center)

	padStyle := lipgloss.NewStyle().
		Background(n.theme.NavBg).
		Width(navWidth)

	activeStyle := lipgloss.NewStyle().
		Background(n.theme.NavActiveBg).
		Foreground(n.theme.NavActiveFg).
		Width(navWidth).
		Align(lipgloss.Center)

	inactiveStyle := lipgloss.NewStyle().
		Background(n.theme.NavBg).
		Foreground(n.theme.NavFg).
		Width(navWidth).
		Align(lipgloss.Center)

	var rows []string
	for _, item := range navItems {
		rows = append(rows, padStyle.Render(""))
		if item == n.Active {
			rows = append(rows, activeStyle.Render(navIcons[item]))
		} else {
			rows = append(rows, inactiveStyle.Render(navIcons[item]))
		}
	}

	used := len(rows)
	remaining := max(0, height-used)
	if remaining > 0 {
		rows = append(rows, baseStyle.Height(remaining).Render(""))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
