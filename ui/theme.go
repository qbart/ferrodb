package ui

import "github.com/charmbracelet/lipgloss"

type Theme struct {
	Bg              lipgloss.Color
	Fg              lipgloss.Color
	Muted           lipgloss.Color
	Accent          lipgloss.Color
	AccentInactive  lipgloss.Color
	Success         lipgloss.Color
	Danger          lipgloss.Color
	Warning         lipgloss.Color
	FooterBg        lipgloss.Color
	FooterFg        lipgloss.Color
	SidebarBg       lipgloss.Color
	NavBg           lipgloss.Color
	NavFg           lipgloss.Color
	NavActiveBg     lipgloss.Color
	NavActiveFg     lipgloss.Color
	SidebarHeaderBg lipgloss.Color
	SidebarHeaderFg lipgloss.Color
}

var (
	Accent         = lipgloss.Color("37")
	AccentInactive = lipgloss.Color("245")
	Black          = lipgloss.Color("234")
)

var DefaultTheme = Theme{
	Bg:              lipgloss.Color("235"),
	Fg:              lipgloss.Color("252"),
	Muted:           lipgloss.Color("243"),
	Accent:          Accent,
	AccentInactive:  AccentInactive,
	Success:         lipgloss.Color("2"),
	Danger:          lipgloss.Color("1"),
	Warning:         lipgloss.Color("3"),
	FooterBg:        Accent,
	FooterFg:        Black,
	SidebarBg:       lipgloss.Color("236"),
	NavBg:           lipgloss.Color("235"),
	NavFg:           Accent,
	NavActiveBg:     Accent,
	NavActiveFg:     Black,
	SidebarHeaderBg: lipgloss.Color("237"),
	SidebarHeaderFg: lipgloss.Color("252"),
}
