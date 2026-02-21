package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"
)

type RowDetail struct {
	Visible   bool
	headers   []string
	values    []string
	colCursor int // which column is "selected" (for copy)
	rowOffset int // line-by-line vertical scroll
	colOffset int // horizontal scroll (rune offset)
	theme     Theme
}

func NewRowDetail(theme Theme) RowDetail {
	return RowDetail{theme: theme}
}

func (r *RowDetail) Open(headers, values []string) {
	r.headers = headers
	r.values = values
	r.colCursor = 0
	r.rowOffset = 0
	r.colOffset = 0
	r.Visible = true
}

func (r *RowDetail) Close() {
	r.Visible = false
}

// ScrollUp scrolls one line up and updates colCursor.
func (r *RowDetail) ScrollUp() {
	if r.rowOffset > 0 {
		r.rowOffset--
		r.syncColCursor()
	}
}

// ScrollDown scrolls one line down and updates colCursor.
func (r *RowDetail) ScrollDown(totalLines, visible int) {
	max := totalLines - visible
	if max > 0 && r.rowOffset < max {
		r.rowOffset++
		r.syncColCursor()
	}
}

// JumpNext jumps to the next column header.
func (r *RowDetail) JumpNext() {
	if r.colCursor < len(r.headers)-1 {
		r.colCursor++
		r.rowOffset = r.lineIndexForCol(r.colCursor)
	}
}

// JumpPrev jumps to the previous column header.
func (r *RowDetail) JumpPrev() {
	if r.colCursor > 0 {
		r.colCursor--
		r.rowOffset = r.lineIndexForCol(r.colCursor)
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

func (r *RowDetail) Copy() {
	if r.colCursor >= len(r.values) {
		return
	}
	if err := clipboard.Init(); err != nil {
		return
	}
	clipboard.Write(clipboard.FmtText, []byte(r.values[r.colCursor]))
}

// syncColCursor updates colCursor to match the current rowOffset.
func (r *RowDetail) syncColCursor() {
	idx := 0
	for i, _ := range r.headers {
		val := ""
		if i < len(r.values) {
			val = r.values[i]
		}
		size := 1 + len(strings.Split(val, "\n")) + 1
		if r.rowOffset < idx+size {
			r.colCursor = i
			return
		}
		idx += size
	}
	if len(r.headers) > 0 {
		r.colCursor = len(r.headers) - 1
	}
}

// lineIndexForCol returns the line index of the header for the given column.
func (r RowDetail) lineIndexForCol(col int) int {
	idx := 0
	for i := 0; i < col && i < len(r.headers); i++ {
		val := ""
		if i < len(r.values) {
			val = r.values[i]
		}
		idx += 1 + len(strings.Split(val, "\n")) + 1
	}
	return idx
}

type detailLine struct {
	text       string
	isHeader   bool
	isSelected bool
}

func (r RowDetail) buildLines() []detailLine {
	var lines []detailLine
	for i, h := range r.headers {
		lines = append(lines, detailLine{text: h, isHeader: true, isSelected: i == r.colCursor})
		val := ""
		if i < len(r.values) {
			val = r.values[i]
		}
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
		Bold(false).
		Width(width)

	selectedHeaderStyle := lipgloss.NewStyle().
		Background(r.theme.NavActiveBg).
		Foreground(r.theme.NavActiveFg).
		Bold(false).
		Width(width)

	valueStyle := lipgloss.NewStyle().
		Background(r.theme.Bg).
		Foreground(r.theme.Fg).
		Width(width).
		Bold(false)

	contentHeight := height - 1 // reserve last line for hint
	end := r.rowOffset + contentHeight
	if end > len(all) {
		end = len(all)
	}
	var visible []detailLine
	if r.rowOffset < len(all) {
		visible = all[r.rowOffset:end]
	}

	var rendered []string
	for _, line := range visible {
		text := cropRunes(line.text, r.colOffset, width)
		switch {
		case line.isHeader && line.isSelected:
			rendered = append(rendered, selectedHeaderStyle.Render(text))
		case line.isHeader:
			rendered = append(rendered, headerStyle.Render(text))
		default:
			rendered = append(rendered, valueStyle.Render(text))
		}
	}

	for len(rendered) < contentHeight {
		rendered = append(rendered, valueStyle.Render(""))
	}

	hintStyle := lipgloss.NewStyle().
		Background(r.theme.FooterBg).
		Foreground(lipgloss.Color("0")).
		Width(width).
		Bold(false)
	rendered = append(rendered, hintStyle.Render("  ↑ ↓  scroll    j k  jump columns    ← →  scroll horizontal    y  copy    Esc  close"))

	return strings.Join(rendered, "\n")
}
