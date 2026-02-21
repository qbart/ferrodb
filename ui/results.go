package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const maxColWidth = 50

type ResultData struct {
	Headers     []string
	Rows        [][]string
	ColumnTypes []string
}

type Results struct {
	data      ResultData
	cursor    int
	rowOffset int
	colOffset int
	widths    []int
	theme     Theme
}

func NewResults(theme Theme) Results {
	return Results{theme: theme}
}

func (r *Results) SetData(data ResultData) {
	r.data = data
	r.cursor = 0
	r.rowOffset = 0
	r.colOffset = 0
	r.widths = computeWidths(data)
}

func (r *Results) MoveUp() {
	if r.cursor > 0 {
		r.cursor--
	}
}

func (r *Results) MoveDown() {
	if r.cursor < len(r.data.Rows)-1 {
		r.cursor++
	}
}

func (r *Results) EnsureVisible(height int) {
	if height <= 0 {
		return
	}
	if r.cursor < r.rowOffset {
		r.rowOffset = r.cursor
	}
	if r.cursor >= r.rowOffset+height {
		r.rowOffset = r.cursor - height + 1
	}
}

func (r *Results) ScrollLeft() {
	if r.colOffset > 0 {
		r.colOffset--
	}
}

func (r *Results) ScrollRight() {
	if r.colOffset < len(r.data.Headers)-1 {
		r.colOffset++
	}
}

func (r Results) HasData() bool {
	return len(r.data.Headers) > 0
}

func (r *Results) Resize(width, height int) {}

func normalizeCell(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	runes := []rune(s)
	if len(runes) > maxColWidth {
		return string(runes[:maxColWidth-1]) + "…"
	}
	return s
}

func runeLen(s string) int {
	return len([]rune(s))
}

func computeWidths(data ResultData) []int {
	widths := make([]int, len(data.Headers))
	for i, h := range data.Headers {
		widths[i] = runeLen(normalizeCell(h))
	}
	for _, row := range data.Rows {
		for i := range widths {
			if i < len(row) {
				w := runeLen(normalizeCell(row[i]))
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}
	return widths
}

func (r Results) View(width, height int, focused bool) string {
	if height <= 0 || width <= 0 {
		return ""
	}

	bgStyle := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Width(width)

	if len(r.data.Headers) == 0 {
		return lipgloss.NewStyle().
			Background(r.theme.Bg).
			Width(width).
			Height(height).
			Render("")
	}

	var headerStyle lipgloss.Style
	if focused {
		headerStyle = lipgloss.NewStyle().
			Background(r.theme.SidebarHeaderBg).
			Foreground(r.theme.Fg).
			Bold(true)
	} else {
		headerStyle = lipgloss.NewStyle().
			Background(r.theme.SidebarBg).
			Foreground(r.theme.Muted).
			Bold(true)
	}

	cellStyle := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Foreground(r.theme.Fg)

	cursorStyle := lipgloss.NewStyle().
		Background(r.theme.NavActiveBg).
		Foreground(r.theme.NavActiveFg)

	cursorBlurStyle := lipgloss.NewStyle().
		Background(r.theme.AccentInactive).
		Foreground(r.theme.FooterFg)

	var lines []string

	lines = append(lines, r.renderRow(r.data.Headers, headerStyle, width))

	dataHeight := max(0, height-1)
	end := r.rowOffset + dataHeight
	if end > len(r.data.Rows) {
		end = len(r.data.Rows)
	}
	for i, row := range r.data.Rows[r.rowOffset:end] {
		actualIdx := r.rowOffset + i
		if actualIdx == r.cursor {
			if focused {
				lines = append(lines, r.renderRow(row, cursorStyle, width))
			} else {
				lines = append(lines, r.renderRow(row, cursorBlurStyle, width))
			}
		} else {
			lines = append(lines, r.renderRow(row, cellStyle, width))
		}
	}

	for len(lines) < height {
		lines = append(lines, bgStyle.Render(""))
	}

	return strings.Join(lines, "\n")
}

func (r Results) renderRow(cells []string, style lipgloss.Style, availWidth int) string {
	if len(r.widths) == 0 || r.colOffset >= len(r.widths) {
		return style.Width(availWidth).Render("")
	}

	var parts []string
	used := 0

	for i := r.colOffset; i < len(r.widths); i++ {
		colW := r.widths[i] + 2

		var cellText string
		if i < len(cells) {
			cellText = normalizeCell(cells[i])
		}
		if runes := []rune(cellText); len(runes) > r.widths[i] {
			cellText = string(runes[:r.widths[i]])
		}

		if used+colW > availWidth {
			remaining := availWidth - used
			if remaining > 1 {
				content := " " + cellText
				if len(content) > remaining {
					content = content[:remaining]
				}
				parts = append(parts, style.Render(content))
				used += len(content)
			}
			break
		}

		cell := fmt.Sprintf(" %-*s ", r.widths[i], cellText)
		parts = append(parts, style.Render(cell))
		used += colW
	}

	if used < availWidth {
		parts = append(parts, style.Width(availWidth-used).Render(""))
	}

	return strings.Join(parts, "")
}
