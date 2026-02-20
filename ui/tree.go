package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var treeSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

type TreeItem struct {
	ID         string
	Label      string
	Children   []TreeItem
	Expandable bool
	Expanded   bool
	Loaded     bool
	loading    bool
}

type Tree struct {
	Items        []TreeItem
	Cursor       int
	Focused      bool
	spinnerFrame int
	theme        Theme
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

// StartLoading marks the cursor item as loading if it is expandable and not yet loaded.
// Returns the full path of IDs from root to the item, and true if loading was started.
func (t *Tree) StartLoading() ([]string, bool) {
	counter := 0
	item, path := findItemWithPath(t.Items, t.Cursor, &counter, nil)
	if item == nil || !item.Expandable || item.Loaded || item.loading {
		return nil, false
	}
	item.loading = true
	t.spinnerFrame = 0
	return path, true
}

func findItemWithPath(items []TreeItem, cursor int, counter *int, parentPath []string) (*TreeItem, []string) {
	for i := range items {
		path := make([]string, len(parentPath)+1)
		copy(path, parentPath)
		path[len(parentPath)] = items[i].ID

		if *counter == cursor {
			return &items[i], path
		}
		*counter++

		if items[i].Expanded {
			if found, foundPath := findItemWithPath(items[i].Children, cursor, counter, path); found != nil {
				return found, foundPath
			}
		}
	}
	return nil, nil
}

func (t *Tree) SetLoaded(ids []string, children []TreeItem) {
	setLoadedByPath(t.Items, ids, children)
}

func setLoadedByPath(items []TreeItem, ids []string, children []TreeItem) bool {
	if len(ids) == 0 {
		return false
	}
	for i := range items {
		if items[i].ID != ids[0] {
			continue
		}
		if len(ids) == 1 {
			items[i].loading = false
			items[i].Loaded = true
			items[i].Expanded = true
			items[i].Children = children
			return true
		}
		return setLoadedByPath(items[i].Children, ids[1:], children)
	}
	return false
}

func (t *Tree) IsLoading() bool {
	return anyLoading(t.Items)
}

func anyLoading(items []TreeItem) bool {
	for _, item := range items {
		if item.loading {
			return true
		}
		if anyLoading(item.Children) {
			return true
		}
	}
	return false
}

func (t *Tree) AdvanceSpinner() {
	t.spinnerFrame++
}

func (t *Tree) Expand() {
	counter := 0
	if item := itemAtCursor(t.Items, t.Cursor, &counter); item != nil && item.Expandable && item.Loaded {
		item.Expanded = true
	}
}

func (t *Tree) Collapse() {
	counter := 0
	if item := itemAtCursor(t.Items, t.Cursor, &counter); item != nil && item.Expandable {
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

// CursorInfo returns the label and depth of the currently selected item.
func (t Tree) CursorInfo() (label string, depth int, ok bool) {
	counter := 0
	return findCursorInfo(t.Items, t.Cursor, &counter, 0)
}

func findCursorInfo(items []TreeItem, cursor int, counter *int, depth int) (string, int, bool) {
	for _, item := range items {
		if *counter == cursor {
			return item.Label, depth, true
		}
		*counter++
		if item.Expanded {
			if label, d, found := findCursorInfo(item.Children, cursor, counter, depth+1); found {
				return label, d, found
			}
		}
	}
	return "", 0, false
}

func truncateLabel(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	return string(runes[:maxWidth-1]) + "…"
}

func (t Tree) View(width, height int) string {
	var lines []string
	idx := 0
	t.renderItems(t.Items, 0, &lines, &idx, width)

	if len(lines) == 0 {
		return ""
	}

	if len(lines) > height {
		lines = lines[:height]
	}

	return strings.Join(lines, "\n")
}

func (t Tree) renderItems(items []TreeItem, depth int, lines *[]string, idx *int, width int) {
	indent := strings.Repeat("  ", depth)

	for _, item := range items {
		isActive := t.Focused && *idx == t.Cursor
		*idx++

		prefix := "  "
		if item.loading {
			prefix = treeSpinnerFrames[t.spinnerFrame%len(treeSpinnerFrames)] + " "
		} else if item.Expandable {
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
				Foreground(t.theme.NavActiveFg).
				Width(width)
		} else {
			style = lipgloss.NewStyle().
				Background(t.theme.SidebarBg).
				Foreground(t.theme.Fg)
		}

		available := width - lipgloss.Width(indent) - 2
		label := truncateLabel(item.Label, available)
		*lines = append(*lines, style.Render(indent+prefix+label))

		if item.Expanded && len(item.Children) > 0 {
			t.renderItems(item.Children, depth+1, lines, idx, width)
		}
	}
}
