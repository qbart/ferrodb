package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Footer struct {
	theme Theme
}

func NewFooter(theme Theme) Footer {
	return Footer{theme: theme}
}

func (f Footer) View(width int) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Background(f.theme.FooterBg).
		Foreground(f.theme.FooterFg)

	left := " ferroDB"
	right := "q: quit "
	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))

	return style.Render(left + strings.Repeat(" ", gap) + right)
}
