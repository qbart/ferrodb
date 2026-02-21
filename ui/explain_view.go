package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/qbart/ferrodb/ferro/plugin"
)

type ExplainView struct {
	result    *plugin.BrowserExplainResult
	err       string
	lines     []explainLine
	rowOffset int
	theme     Theme
}

type explainLine struct {
	text  string
	color lipgloss.Color
	bold  bool
}

func ParseExplainResult(data ResultData) (plugin.BrowserExplainResult, error) {
	if len(data.Rows) == 0 || len(data.Headers) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf("Query result does not match the explain output")
	}
	// Join all row values from the first column into one JSON string
	var sb strings.Builder
	for _, row := range data.Rows {
		if len(row) > 0 {
			sb.WriteString(row[0])
		}
	}
	raw := sb.String()
	var results []plugin.BrowserExplainResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return plugin.BrowserExplainResult{}, fmt.Errorf("Query result does not match the explain output")
	}
	if len(results) == 0 {
		return plugin.BrowserExplainResult{}, fmt.Errorf("Query result does not match the explain output")
	}
	return results[0], nil
}

func NewExplainView(theme Theme) ExplainView {
	return ExplainView{theme: theme}
}

func (e *ExplainView) SetResult(result plugin.BrowserExplainResult) {
	e.result = &result
	e.err = ""
	e.rowOffset = 0
	e.lines = buildExplainLines(result)
}

func (e *ExplainView) SetError(msg string) {
	e.result = nil
	e.err = msg
	e.lines = nil
	e.rowOffset = 0
}

func (e *ExplainView) Clear() {
	e.result = nil
	e.err = ""
	e.lines = nil
	e.rowOffset = 0
}

func (e *ExplainView) ScrollUp() {
	if e.rowOffset > 0 {
		e.rowOffset--
	}
}

func (e *ExplainView) ScrollDown(height int) {
	max := len(e.lines) - height
	if max > 0 && e.rowOffset < max {
		e.rowOffset++
	}
}

func (e ExplainView) HasContent() bool {
	return e.result != nil || e.err != ""
}

func buildExplainLines(result plugin.BrowserExplainResult) []explainLine {
	var lines []explainLine
	lines = buildPlanLines(lines, result.Plan, 0)
	lines = append(lines, explainLine{})
	lines = append(lines, explainLine{
		text:  fmt.Sprintf("  Planning Time:   %.3f ms", result.PlanningTime),
		color: lipgloss.Color("243"),
	})
	lines = append(lines, explainLine{
		text:  fmt.Sprintf("  Execution Time:  %.3f ms", result.ExecutionTime),
		color: lipgloss.Color("243"),
	})
	return lines
}

func buildPlanLines(lines []explainLine, plan plugin.BrowserExplainPlan, depth int) []explainLine {
	indent := strings.Repeat("  ", depth)

	// node header
	nodeLabel := plan.NodeType
	if plan.RelationName != "" {
		nodeLabel += " on " + plan.RelationName
		if plan.Alias != "" && plan.Alias != plan.RelationName {
			nodeLabel += " (" + plan.Alias + ")"
		}
	}

	lines = append(lines, explainLine{
		text:  indent + "› " + nodeLabel,
		color: lipgloss.Color("6"), // accent
		bold:  true,
	})

	// cost vs actual
	costColor := costColor(plan.TotalCost)
	lines = append(lines, explainLine{
		text: fmt.Sprintf("%s  cost: %.2f..%.2f  rows: %d  width: %d",
			indent+"  ", plan.StartupCost, plan.TotalCost, plan.PlanRows, plan.PlanWidth),
		color: lipgloss.Color("243"),
	})

	if plan.ActualTotalTime > 0 || plan.ActualRows > 0 {
		ratio := ""
		if plan.PlanRows > 0 {
			r := float64(plan.ActualRows) / float64(plan.PlanRows)
			if r > 10 || r < 0.1 {
				ratio = fmt.Sprintf("  ⚠ est/actual rows ratio: %.1fx", r)
			}
		}
		lines = append(lines, explainLine{
			text: fmt.Sprintf("%s  actual: %.3f..%.3f ms  rows: %d  loops: %d%s",
				indent+"  ", plan.ActualStartupTime, plan.ActualTotalTime,
				plan.ActualRows, plan.ActualLoops, ratio),
			color: costColor,
		})
	}

	// buffers
	if plan.SharedHitBlocks+plan.SharedReadBlocks > 0 {
		lines = append(lines, explainLine{
			text: fmt.Sprintf("%s  buffers: hit=%d  read=%d  dirtied=%d  written=%d",
				indent+"  ",
				plan.SharedHitBlocks, plan.SharedReadBlocks,
				plan.SharedDirtiedBlocks, plan.SharedWrittenBlocks),
			color: lipgloss.Color("243"),
		})
	}

	for _, child := range plan.Plans {
		lines = buildPlanLines(lines, child, depth+1)
	}
	return lines
}

func costColor(totalCost float64) lipgloss.Color {
	switch {
	case totalCost > 10000:
		return lipgloss.Color("1") // red
	case totalCost > 1000:
		return lipgloss.Color("3") // yellow
	default:
		return lipgloss.Color("2") // green
	}
}

func (e ExplainView) View(width, height int) string {
	bg := lipgloss.NewStyle().Background(e.theme.Bg).Width(width)

	if e.err != "" {
		errStyle := lipgloss.NewStyle().
			Background(e.theme.Bg).
			Foreground(lipgloss.Color("1")).
			Width(width)
		var rows []string
		rows = append(rows, bg.Render(""))
		rows = append(rows, errStyle.Render("  "+e.err))
		for len(rows) < height {
			rows = append(rows, bg.Render(""))
		}
		return strings.Join(rows, "\n")
	}

	if len(e.lines) == 0 {
		var rows []string
		for len(rows) < height {
			rows = append(rows, bg.Render(""))
		}
		return strings.Join(rows, "\n")
	}

	end := e.rowOffset + height
	if end > len(e.lines) {
		end = len(e.lines)
	}
	visible := e.lines[e.rowOffset:end]

	var rows []string
	for _, line := range visible {
		if line.text == "" {
			rows = append(rows, bg.Render(""))
			continue
		}
		style := lipgloss.NewStyle().
			Background(e.theme.Bg).
			Foreground(line.color).
			Width(width)
		if line.bold {
			style = style.Bold(true)
		}
		rows = append(rows, style.Render(line.text))
	}
	for len(rows) < height {
		rows = append(rows, bg.Render(""))
	}
	return strings.Join(rows, "\n")
}
