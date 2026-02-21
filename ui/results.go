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
	r.rowOffset = 0
	r.colOffset = 0
	r.widths = computeWidths(data)
}

func (r *Results) ScrollUp() {
	if r.rowOffset > 0 {
		r.rowOffset--
	}
}

func (r *Results) ScrollDown() {
	if r.rowOffset < len(r.data.Rows)-1 {
		r.rowOffset++
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
		w := runeLen(normalizeCell(h))
		widths[i] = w
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

	headerBg := r.theme.AccentInactive
	if focused {
		headerBg = r.theme.FooterBg
	}

	headerStyle := lipgloss.NewStyle().
		Background(headerBg).
		Foreground(r.theme.FooterFg).
		Bold(true)

	cellStyle := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Foreground(r.theme.Fg)

	var lines []string

	lines = append(lines, r.renderRow(r.data.Headers, headerStyle, width))

	dataHeight := max(0, height-1)
	end := r.rowOffset + dataHeight
	if end > len(r.data.Rows) {
		end = len(r.data.Rows)
	}
	for _, row := range r.data.Rows[r.rowOffset:end] {
		lines = append(lines, r.renderRow(row, cellStyle, width))
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
