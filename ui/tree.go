package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type TreeItem struct {
	ID       string
	Label    string
	Children []TreeItem
	Expanded bool
}

type Tree struct {
	Items   []TreeItem
	Cursor  int
	Focused bool
	theme   Theme
}

func NewTree(theme Theme) Tree {
	return Tree{theme: theme}
}

func (t *Tree) MoveUp() {
	if t.Cursor > 0 {
		t.Cursor--
	}
}

func (t *Tree) MoveDown() {
	if t.Cursor < visibleCount(t.Items)-1 {
		t.Cursor++
	}
}

func (t *Tree) Expand() {
	counter := 0
	if item := itemAtCursor(t.Items, t.Cursor, &counter); item != nil && len(item.Children) > 0 {
		item.Expanded = true
	}
}

func (t *Tree) Collapse() {
	counter := 0
	if item := itemAtCursor(t.Items, t.Cursor, &counter); item != nil {
		item.Expanded = false
	}
}

func visibleCount(items []TreeItem) int {
	n := 0
	for _, item := range items {
		n++
		if item.Expanded {
			n += visibleCount(item.Children)
		}
	}
	return n
}

func itemAtCursor(items []TreeItem, cursor int, counter *int) *TreeItem {
	for i := range items {
		if *counter == cursor {
			return &items[i]
		}
		*counter++
		if items[i].Expanded {
			if found := itemAtCursor(items[i].Children, cursor, counter); found != nil {
				return found
			}
		}
	}
	return nil
}

func (t Tree) View(width, height int) string {
	var lines []string
	idx := 0
	t.renderItems(t.Items, 0, &lines, &idx)

	if len(lines) == 0 {
		return ""
	}

	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

func (t Tree) renderItems(items []TreeItem, depth int, lines *[]string, idx *int) {
	indent := strings.Repeat("  ", depth)

	for _, item := range items {
		isActive := t.Focused && *idx == t.Cursor
		*idx++

		prefix := "  "
		if len(item.Children) > 0 {
			if item.Expanded {
				prefix = "▼ "
			} else {
				prefix = "▶ "
			}
		}

		var style lipgloss.Style
		if isActive {
			style = lipgloss.NewStyle().
				Background(t.theme.NavActiveBg).
				Foreground(t.theme.NavActiveFg)
		} else {
			style = lipgloss.NewStyle().
				Background(t.theme.SidebarBg).
				Foreground(t.theme.Fg)
		}

		*lines = append(*lines, style.Render(indent+prefix+item.Label))

		if item.Expanded && len(item.Children) > 0 {
			t.renderItems(item.Children, depth+1, lines, idx)
		}
	}
}
