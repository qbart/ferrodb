package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type Results struct {
	vp    viewport.Model
	theme Theme
}

func NewResults(theme Theme) Results {
	return Results{theme: theme}
}

func (r *Results) SetContent(s string) {
	r.vp.SetContent(s)
}

func (r *Results) Resize(width, height int) {
	r.vp.Width = width
	r.vp.Height = height
}

func (r Results) View(width, height int) string {
	style := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Foreground(r.theme.Fg).
		Width(width).
		Height(height)

	return style.Render(r.vp.View())
}
