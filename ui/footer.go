package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"◐", "◓", "◑", "◒"}

type Footer struct {
	QueryStart time.Time
	QueryDone  bool
	QueryMs    int64
	Running    bool
	tick       int
	theme      Theme
}

func NewFooter(theme Theme) Footer {
	return Footer{theme: theme}
}

func (f *Footer) Tick() {
	f.tick++
}

func (f Footer) View(width int, label string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Background(f.theme.FooterBg).
		Foreground(f.theme.FooterFg)

	left := " " + label

	var right string
	if f.Running {
		elapsed := time.Since(f.QueryStart).Milliseconds()
		frame := spinnerFrames[f.tick%len(spinnerFrames)]
		right = fmt.Sprintf("%s %dms ", frame, elapsed)
	} else if f.QueryDone {
		right = fmt.Sprintf("%dms ", f.QueryMs)
	} else {
		right = ""
	}

	gap := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))

	return style.Render(left + strings.Repeat(" ", gap) + right)
}
