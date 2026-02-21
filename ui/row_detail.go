package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type RowDetail struct {
	Visible   bool
	headers   []string
	values    []string
	rowOffset int
	colOffset int
	theme     Theme
}

func NewRowDetail(theme Theme) RowDetail {
	return RowDetail{theme: theme}
}

func (r *RowDetail) Open(headers, values []string) {
	r.headers = headers
	r.values = values
	r.rowOffset = 0
	r.colOffset = 0
	r.Visible = true
}

func (r *RowDetail) Close() {
	r.Visible = false
}

func (r *RowDetail) ScrollUp() {
	if r.rowOffset > 0 {
		r.rowOffset--
	}
}

func (r *RowDetail) ScrollDown(totalLines, visible int) {
	max := totalLines - visible
	if max > 0 && r.rowOffset < max {
		r.rowOffset++
	}
}

func (r *RowDetail) ScrollLeft() {
	if r.colOffset > 0 {
		r.colOffset--
	}
}

func (r *RowDetail) ScrollRight() {
	r.colOffset++
}

type detailLine struct {
	text     string
	isHeader bool
}

func (r RowDetail) buildLines() []detailLine {
	var lines []detailLine
	for i, h := range r.headers {
		lines = append(lines, detailLine{text: h, isHeader: true})
		val := ""
		if i < len(r.values) {
			val = r.values[i]
		}
		// split on newlines in the value
		for _, part := range strings.Split(val, "\n") {
			lines = append(lines, detailLine{text: part, isHeader: false})
		}
		lines = append(lines, detailLine{text: "", isHeader: false})
	}
	return lines
}

func cropRunes(s string, offset, width int) string {
	runes := []rune(s)
	if offset >= len(runes) {
		return ""
	}
	runes = runes[offset:]
	if len(runes) > width {
		runes = runes[:width]
	}
	return string(runes)
}

func (r RowDetail) View(width, height int) string {
	if !r.Visible {
		return ""
	}

	all := r.buildLines()

	headerStyle := lipgloss.NewStyle().
		Background(r.theme.SidebarBg).
		Foreground(r.theme.Accent).
		Bold(true).
		Width(width)

	valueStyle := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Foreground(r.theme.Fg).
		Width(width)

	end := r.rowOffset + height
	if end > len(all) {
		end = len(all)
	}
	visible := all
	if r.rowOffset < len(all) {
		visible = all[r.rowOffset:end]
	} else {
		visible = nil
	}

	var rendered []string
	for _, line := range visible {
		text := cropRunes(line.text, r.colOffset, width)
		if line.isHeader {
			rendered = append(rendered, headerStyle.Render(text))
		} else {
			rendered = append(rendered, valueStyle.Render(text))
		}
	}

	// pad remaining lines to fill screen
	for len(rendered) < height {
		rendered = append(rendered, valueStyle.Render(""))
	}

	return strings.Join(rendered, "\n")
}
