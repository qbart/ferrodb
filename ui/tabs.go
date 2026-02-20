package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TabItem struct {
	Title    string
	Content  string
	Modified bool
}

type Tabs struct {
	Items   []TabItem
	Active  int
	Focused bool
	theme   Theme
}

func NewTabs(theme Theme) Tabs {
	return Tabs{theme: theme}
}

func (t Tabs) View(width int) string {
	var tabs []string
	for i, item := range t.Items {
		var style lipgloss.Style
		if i == t.Active {
			activeBg := t.theme.FooterBg
			if !t.Focused {
				activeBg = t.theme.AccentInactive
			}
			style = lipgloss.NewStyle().
				Background(activeBg).
				Foreground(t.theme.FooterFg).
				Bold(true).
				Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().
				Background(t.theme.Bg).
				Foreground(t.theme.Muted).
				Padding(0, 1)
		}

		rendered := style.Render(item.Title)

		if item.Modified {
			bg := t.theme.Bg
			if i == t.Active {
				bg = t.theme.FooterBg
				if !t.Focused {
					bg = t.theme.AccentInactive
				}
			}
			dot := lipgloss.NewStyle().
				Foreground(t.theme.Danger).
				Background(bg).
				Render(" •")
			rendered = rendered + dot
		}

		tabs = append(tabs, rendered)
	}

	tabLine := strings.Join(tabs, "")
	tabLineWidth := lipgloss.Width(tabLine)

	gap := max(0, width-tabLineWidth)
	filler := lipgloss.NewStyle().
		Background(t.theme.SidebarHeaderBg).
		Width(gap).
		Render("")

	return tabLine + filler
}
