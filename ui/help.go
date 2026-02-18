package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var shortcuts = []struct {
	key  string
	desc string
}{
	{"F1", "Toggle help"},
	{"Ctrl+C", "Quit"},
	{"Ctrl+\\", "Toggle sidebar"},
	{"Ctrl+T", "New tab"},
	{"Ctrl+R", "Run query"},
	{"Tab", "Next tab"},
	{"Shift+Tab", "Previous tab"},
}

type Help struct {
	Visible bool
	theme   Theme
}

func NewHelp(theme Theme) Help {
	return Help{theme: theme}
}

func (h Help) View(width, height int) string {
	if !h.Visible {
		return ""
	}

	boxWidth := 36
	boxHeight := len(shortcuts) + 4

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.Fg).
		Background(h.theme.SidebarBg)

	keyStyle := lipgloss.NewStyle().
		Foreground(h.theme.Accent).
		Background(h.theme.SidebarBg).
		Width(14)

	descStyle := lipgloss.NewStyle().
		Foreground(h.theme.Fg).
		Background(h.theme.SidebarBg)

	var lines []string
	lines = append(lines, titleStyle.Render(" Keyboard Shortcuts"))
	lines = append(lines, "")
	for _, s := range shortcuts {
		line := " " + keyStyle.Render(s.key) + descStyle.Render(s.desc)
		lines = append(lines, line)
	}
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Background(h.theme.SidebarBg).
		Width(boxWidth).
		Height(boxHeight).
		Padding(1, 2)

	box := boxStyle.Render(content)

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceBackground(h.theme.Bg),
	)
}
