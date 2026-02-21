package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var shortcuts = []struct {
	key     string
	desc    string
	section string
}{
	{section: "Global"},
	{"F1", "Toggle help", ""},
	{"Ctrl+c", "Quit", ""},
	{"Tab / Shift+Tab", "Cycle focus", ""},
	{"Ctrl+\\", "Toggle sidebar", ""},
	{"Ctrl+g", "Toggle Data / Explain mode", ""},
	{"Ctrl+t", "New tab", ""},
	{"Ctrl+w", "Close tab", ""},
	{"Ctrl+r", "Run query", ""},
	{"Ctrl+h / Ctrl+Left", "Previous tab", ""},
	{"Ctrl+l / Ctrl+Right", "Next tab", ""},
	{section: "Tree"},
	{"↓ ↑ / j k", "Navigate", ""},
	{"→ / l", "Expand / load", ""},
	{"← / h", "Collapse parent", ""},
	{"Enter", "Open & run query", ""},
	{"Shift+r", "Reload tree", ""},
	{section: "Results"},
	{"↓ ↑ / j k", "Move row cursor", ""},
	{"← → / h l", "Scroll columns", ""},
	{"Enter", "View row detail", ""},
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

	boxWidth := 84
	boxHeight := len(shortcuts) + 4

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.Fg).
		Background(h.theme.SidebarBg)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(h.theme.Accent).
		Background(h.theme.SidebarBg)

	innerWidth := boxWidth - 4 // subtract Padding(1,2) left+right
	keyWidth := innerWidth / 2
	keyStyle := lipgloss.NewStyle().
		Foreground(h.theme.Fg).
		Background(h.theme.SidebarBg).
		Width(keyWidth)

	descStyle := lipgloss.NewStyle().
		Foreground(h.theme.Muted).
		Background(h.theme.SidebarBg)

	var lines []string
	lines = append(lines, titleStyle.Render(" Keyboard Shortcuts"))
	lines = append(lines, "")
	for _, s := range shortcuts {
		if s.section != "" {
			lines = append(lines, " "+sectionStyle.Render(s.section))
			continue
		}
		line := "   " + keyStyle.Render(s.key) + descStyle.Render(s.desc)
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
