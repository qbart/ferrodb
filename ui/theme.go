package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Bg             lipgloss.Color
	Fg             lipgloss.Color
	Muted          lipgloss.Color
	Accent         lipgloss.Color
	AccentInactive lipgloss.Color
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
	SidebarHeaderBg lipgloss.Color
	SidebarHeaderFg lipgloss.Color
}

var DefaultTheme = Theme{
	Bg:            lipgloss.Color("235"),
	Fg:            lipgloss.Color("252"),
	Muted:         lipgloss.Color("243"),
	Accent:         lipgloss.Color("6"),
	AccentInactive: lipgloss.Color("37"),
	Success:       lipgloss.Color("2"),
	Danger:        lipgloss.Color("1"),
	Warning:       lipgloss.Color("3"),
	FooterBg:      lipgloss.Color("30"),
	FooterFg:      lipgloss.Color("234"),
	SidebarBg:     lipgloss.Color("236"),
	NavBg:           lipgloss.Color("235"),
	NavFg:           lipgloss.Color("116"),
	NavActiveBg:     lipgloss.Color("30"),
	NavActiveFg:     lipgloss.Color("234"),
	SidebarHeaderBg: lipgloss.Color("237"),
	SidebarHeaderFg: lipgloss.Color("252"),
}
