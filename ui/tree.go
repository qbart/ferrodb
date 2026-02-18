package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TreeItem struct {
	Label    string
	Children []TreeItem
	Expanded bool
}

type Tree struct {
	Items  []TreeItem
	Cursor int
	theme  Theme
}

func NewTree(theme Theme) Tree {
	return Tree{theme: theme}
}

func (t Tree) View(width, height int) string {
	var lines []string
	t.renderItems(t.Items, 0, &lines)

	if len(lines) == 0 {
		return ""
	}

	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

func (t Tree) renderItems(items []TreeItem, depth int, lines *[]string) {
	indent := strings.Repeat("  ", depth)

	for _, item := range items {
		prefix := "  "
		if len(item.Children) > 0 {
			if item.Expanded {
				prefix = "▼ "
			} else {
				prefix = "▶ "
			}
		}

		style := lipgloss.NewStyle().
			Background(t.theme.SidebarBg).
			Foreground(t.theme.Fg)

		line := style.Render(indent + prefix + item.Label)
		*lines = append(*lines, line)

		if item.Expanded && len(item.Children) > 0 {
			t.renderItems(item.Children, depth+1, lines)
		}
	}
}
