package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Bg            lipgloss.Color
	Fg            lipgloss.Color
	Muted         lipgloss.Color
	Accent        lipgloss.Color
	Success       lipgloss.Color
	Danger        lipgloss.Color
	Warning       lipgloss.Color
	FooterBg      lipgloss.Color
	FooterFg      lipgloss.Color
	SidebarBg     lipgloss.Color
	NavBg           lipgloss.Color
	NavFg           lipgloss.Color
	NavActiveBg     lipgloss.Color
	NavActiveFg     lipgloss.Color
}

var DefaultTheme = Theme{
	Bg:            lipgloss.Color("235"),
	Fg:            lipgloss.Color("252"),
	Muted:         lipgloss.Color("243"),
	Accent:        lipgloss.Color("6"),
	Success:       lipgloss.Color("2"),
	Danger:        lipgloss.Color("1"),
	Warning:       lipgloss.Color("3"),
	FooterBg:      lipgloss.Color("134"),
	FooterFg:      lipgloss.Color("234"),
	SidebarBg:     lipgloss.Color("236"),
	NavBg:           lipgloss.Color("235"),
	NavFg:           lipgloss.Color("183"),
	NavActiveBg:     lipgloss.Color("134"),
	NavActiveFg:     lipgloss.Color("234"),
}
