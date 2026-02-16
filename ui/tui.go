package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type navItem int

const (
	navDatabase navItem = iota
	navFavourites
)

var navIcons = map[navItem]string{
	navDatabase:   "\u26C1",
	navFavourites: "\u2605",
}

const navWidth = 3

type TUI struct {
	width     int
	height    int
	theme     Theme
	activeNav navItem
}

func New() TUI {
	return TUI{
		theme:     DefaultTheme,
		activeNav: navDatabase,
	}
}

func (t TUI) Init() tea.Cmd {
	return nil
}

func (t TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return t, tea.Quit
		case "1":
			t.activeNav = navDatabase
			return t, nil
		case "2":
			t.activeNav = navFavourites
			return t, nil
		}
	}
	return t, nil
}

func (t TUI) View() string {
	if t.height == 0 || t.width == 0 {
		return ""
	}

	footerStyle := lipgloss.NewStyle().
		Bold(true).
		Background(t.theme.FooterBg).
		Foreground(t.theme.FooterFg)

	// footer
	left := " ferroDB"
	right := "q: quit "
	gap := max(0, t.width-lipgloss.Width(left)-lipgloss.Width(right))
	footer := footerStyle.Render(left + strings.Repeat(" ", gap) + right)

	// main
	mainHeight := t.height - 1
	sidebarWidth := t.width * 25 / 100
	contentWidth := t.width - sidebarWidth
	treeWidth := sidebarWidth - navWidth

	// nav
	nav := t.renderNav(mainHeight)

	// tree
	treeStyle := lipgloss.NewStyle().
		Background(t.theme.SidebarBg).
		Foreground(t.theme.Fg).
		Width(treeWidth).
		Height(mainHeight)
	tree := treeStyle.Render(lipgloss.NewStyle().
		Foreground(t.theme.Muted).
		Background(t.theme.SidebarBg).
		Render(" Tree"))

	// content
	contentStyle := lipgloss.NewStyle().
		Background(t.theme.Bg).
		Foreground(t.theme.Fg).
		Width(contentWidth).
		Height(mainHeight)
	content := contentStyle.Render(lipgloss.NewStyle().
		Foreground(t.theme.Muted).
		Background(t.theme.Bg).
		Render(" Content"))

	main := lipgloss.JoinHorizontal(lipgloss.Top, nav, tree, content)

	return main + "\n" + footer
}

func (t TUI) renderNav(height int) string {
	navStyle := lipgloss.NewStyle().
		Background(t.theme.NavBg).
		Width(navWidth).
		Align(lipgloss.Center)

	padStyle := lipgloss.NewStyle().
		Background(t.theme.NavBg).
		Width(navWidth)

	activeStyle := lipgloss.NewStyle().
		Background(t.theme.NavActiveBg).
		Foreground(t.theme.NavActiveFg).
		Width(navWidth).
		Align(lipgloss.Center)

	inactiveStyle := lipgloss.NewStyle().
		Background(t.theme.NavBg).
		Foreground(t.theme.NavFg).
		Width(navWidth).
		Align(lipgloss.Center)

	items := []navItem{navDatabase, navFavourites}
	var rows []string
	for _, item := range items {
		rows = append(rows, padStyle.Render(""))
		if item == t.activeNav {
			rows = append(rows, activeStyle.Render(navIcons[item]))
		} else {
			rows = append(rows, inactiveStyle.Render(navIcons[item]))
		}
	}

	used := len(rows) // padding + icon rows already counted
	remaining := max(0, height-used)
	if remaining > 0 {
		rows = append(rows, navStyle.Height(remaining).Render(""))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
