package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type ExplainView struct {
	result    *plugin.BrowserExplainResult
	err       string
	rowOffset int
	theme     Theme
}

type explainLine struct {
	text     string
	color    lipgloss.Color
	bold     bool
	bordered bool   // true = render with │ frame; indent is the nodePrefix
	indent   string // nodePrefix placed before │ (may be "" for root)
}

func NewExplainView(theme Theme) ExplainView {
	return ExplainView{theme: theme}
}

func (e *ExplainView) SetResult(result plugin.BrowserExplainResult) {
	e.result = &result
	e.err = ""
	e.rowOffset = 0
}

func (e *ExplainView) SetError(msg string) {
	e.result = nil
	e.err = msg
	e.rowOffset = 0
}

func (e *ExplainView) Clear() {
	e.result = nil
	e.err = ""
	e.rowOffset = 0
}

func (e *ExplainView) ScrollUp() {
	if e.rowOffset > 0 {
		e.rowOffset--
	}
}

func (e *ExplainView) ScrollDown(height, width int) {
	lines := e.buildLines(width)
	max := len(lines) - height
	if max > 0 && e.rowOffset < max {
		e.rowOffset++
	}
}

func (e ExplainView) HasContent() bool {
	return e.result != nil || e.err != ""
}

func (e ExplainView) buildLines(width int) []explainLine {
	if e.result == nil {
		return nil
	}
	var lines []explainLine
	lines = buildNodeBox(lines, e.result.Root, "", 0, true, true, width)
	if len(e.result.SummaryLines) > 0 {
		lines = append(lines, explainLine{})
		for _, sl := range e.result.SummaryLines {
			color := lipgloss.Color("243")
			if sl.Highlight {
				color = lipgloss.Color("3")
			}
			lines = append(lines, explainLine{text: "  " + sl.Text, color: color})
		}
	}
	return lines
}

// buildNodeBox renders a BrowserExplainNode as a Unicode box with connecting arrows.
//
//	prefix:     prefix string for content lines at this node's level (inherited from parent)
//	nodeDepth:  visual depth — each level is 4 terminal cells wide
//	isLast:     true when this is the last child among its siblings
//	isRoot:     true for the top-level node (no connector arrow)
func buildNodeBox(lines []explainLine, node plugin.BrowserExplainNode, prefix string, nodeDepth int, isLast bool, isRoot bool, totalWidth int) []explainLine {
	var nodePrefix string
	if isRoot {
		nodePrefix = ""
	} else if isLast {
		nodePrefix = prefix + "    "
	} else {
		nodePrefix = prefix + "│   "
	}

	boxContentWidth := totalWidth - nodeDepth*4 - 4
	if boxContentWidth < 20 {
		boxContentWidth = 20
	}

	const borderColor = lipgloss.Color("240")
	hLine := strings.Repeat("─", boxContentWidth+2)

	border := func(s string) explainLine {
		return explainLine{text: s, color: borderColor}
	}
	cell := func(text string, color lipgloss.Color, bold bool) explainLine {
		return explainLine{
			text:     explainPad(text, boxContentWidth),
			color:    color,
			bold:     bold,
			bordered: true,
			indent:   nodePrefix,
		}
	}

	// Box top — root has no arrow; children get ├─▶ or └─▶
	if isRoot {
		lines = append(lines, border(nodePrefix+"┌"+hLine+"┐"))
	} else {
		arrow := "├─▶ "
		if isLast {
			arrow = "└─▶ "
		}
		lines = append(lines, border(prefix+arrow+"┌"+hLine+"┐"))
	}

	// Node name — always bold accent
	lines = append(lines, cell(node.Name, lipgloss.Color("6"), true))

	// Content lines — highlighted lines stand out, rest are muted
	for _, line := range node.Lines {
		color := lipgloss.Color("243")
		if line.Highlight {
			color = lipgloss.Color("3")
		}
		lines = append(lines, cell(line.Text, color, false))
	}

	// Box bottom
	lines = append(lines, border(nodePrefix+"└"+hLine+"┘"))

	// Children
	for i, child := range node.Children {
		childIsLast := i == len(node.Children)-1
		lines = append(lines, border(nodePrefix+"│"))
		lines = buildNodeBox(lines, child, nodePrefix, nodeDepth+1, childIsLast, false, totalWidth)
	}

	return lines
}

func explainPad(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		if width > 1 {
			return string(runes[:width-1]) + "…"
		}
		return string(runes[:width])
	}
	return s + strings.Repeat(" ", width-len(runes))
}

func (e ExplainView) View(width, height int) string {
	bg := lipgloss.NewStyle().Background(e.theme.Bg).Width(width)

	if e.err != "" {
		errStyle := lipgloss.NewStyle().
			Background(e.theme.Bg).
			Foreground(lipgloss.Color("1")).
			Width(width)
		detailStyle := lipgloss.NewStyle().
			Background(e.theme.Bg).
			Foreground(lipgloss.Color("243")).
			Width(width)
		parts := strings.SplitN(e.err, "\n", 2)
		var rows []string
		rows = append(rows, bg.Render(""))
		rows = append(rows, errStyle.Render("  "+parts[0]))
		if len(parts) == 2 {
			rows = append(rows, bg.Render(""))
			rows = append(rows, detailStyle.Render("  "+parts[1]))
		}
		for len(rows) < height {
			rows = append(rows, bg.Render(""))
		}
		return strings.Join(rows, "\n")
	}

	lines := e.buildLines(width)

	if len(lines) == 0 {
		var rows []string
		for len(rows) < height {
			rows = append(rows, bg.Render(""))
		}
		return strings.Join(rows, "\n")
	}

	offset := e.rowOffset
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[offset:end]

	borderStyle := lipgloss.NewStyle().Background(e.theme.Bg).Foreground(lipgloss.Color("240"))

	var rows []string
	for _, line := range visible {
		if line.text == "" {
			rows = append(rows, bg.Render(""))
			continue
		}
		if !line.bordered {
			rows = append(rows, lipgloss.NewStyle().Background(e.theme.Bg).Foreground(line.color).Width(width).Render(line.text))
			continue
		}
		// Bordered content line: indent+│ in border color, text in content color, │ in border color
		contentStyle := lipgloss.NewStyle().Background(e.theme.Bg).Foreground(line.color)
		if line.bold {
			contentStyle = contentStyle.Bold(true)
		}
		rows = append(rows, borderStyle.Render(line.indent+"│ ")+contentStyle.Render(line.text)+borderStyle.Render(" │"))
	}
	for len(rows) < height {
		rows = append(rows, bg.Render(""))
	}
	return strings.Join(rows, "\n")
}
