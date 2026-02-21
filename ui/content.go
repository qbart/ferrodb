package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Content struct {
	Tabs           Tabs
	textareas      []textarea.Model
	results        []Results
	resultsFocused bool
	width          int
	height         int
	theme          Theme
}

func NewContent(theme Theme) Content {
	return Content{
		Tabs:  NewTabs(theme),
		theme: theme,
	}
}

func (c *Content) newTextarea(value string) textarea.Model {
	ta := textarea.New()
	ta.SetValue(value)
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	ta.Prompt = ""
	ta.FocusedStyle.Base = lipgloss.NewStyle().Background(c.theme.Bg)
	ta.BlurredStyle.Base = lipgloss.NewStyle().Background(c.theme.Bg)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(c.theme.SidebarBg)
	ta.FocusedStyle.LineNumber = lipgloss.NewStyle().Foreground(c.theme.Muted).Background(c.theme.Bg)
	ta.BlurredStyle.LineNumber = lipgloss.NewStyle().Foreground(c.theme.Muted).Background(c.theme.Bg)
	ta.FocusedStyle.CursorLineNumber = lipgloss.NewStyle().Foreground(c.theme.Fg).Background(c.theme.SidebarBg)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(c.theme.Fg).Background(c.theme.Bg)
	ta.BlurredStyle.Text = lipgloss.NewStyle().Foreground(c.theme.Fg).Background(c.theme.Bg)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(c.theme.Muted).Background(c.theme.Bg)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(c.theme.Muted).Background(c.theme.Bg)
	ta.Cursor.Style = lipgloss.NewStyle().Background(c.theme.Accent)
	return ta
}

func (c *Content) SetTabs(tabs []TabItem) {
	c.Tabs.Items = tabs
	c.textareas = make([]textarea.Model, len(tabs))
	c.results = make([]Results, len(tabs))
	for i, tab := range tabs {
		c.textareas[i] = c.newTextarea(tab.Content)
		c.results[i] = NewResults(c.theme)
	}
}

func (c *Content) AddTab() {
	n := len(c.Tabs.Items) + 1
	tab := TabItem{Title: fmt.Sprintf("%d", n)}
	c.Tabs.Items = append(c.Tabs.Items, tab)

	ta := c.newTextarea("")
	bodyHeight := max(0, c.height-1)
	topHeight := bodyHeight / 2
	bottomHeight := bodyHeight - topHeight
	ta.SetWidth(c.width)
	ta.SetHeight(topHeight)
	c.textareas = append(c.textareas, ta)

	r := NewResults(c.theme)
	r.Resize(c.width, bottomHeight)
	c.results = append(c.results, r)

	focused := c.Tabs.Focused
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
	c.Tabs.Active = len(c.Tabs.Items) - 1
	if focused {
		c.textareas[c.Tabs.Active].Focus()
	}
	c.Tabs.Focused = focused
}

func (c *Content) Resize(width, height int) {
	c.width = width
	c.height = height
	bodyHeight := max(0, height-1)
	topHeight := bodyHeight / 2
	bottomHeight := bodyHeight - topHeight
	for i := range c.textareas {
		c.textareas[i].SetWidth(width)
		c.textareas[i].SetHeight(topHeight)
		c.results[i].Resize(width, bottomHeight)
	}
}

func (c *Content) Focus() {
	c.Tabs.Focused = true
	c.resultsFocused = false
	if len(c.textareas) > 0 && c.Tabs.Active < len(c.textareas) {
		c.textareas[c.Tabs.Active].Focus()
	}
}

func (c *Content) Blur() {
	c.Tabs.Focused = false
	c.resultsFocused = false
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
}

func (c *Content) FocusResults() {
	c.Tabs.Focused = false
	c.resultsFocused = true
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
}

func (c *Content) BlurResults() {
	c.resultsFocused = false
}

func (c *Content) ResultsMoveUp() {
	if c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].MoveUp()
	}
}

func (c *Content) ResultsMoveDown() {
	if c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].MoveDown()
	}
}

func (c *Content) ResultsEnsureVisible(height int) {
	if c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].EnsureVisible(height)
	}
}

func (c *Content) ResultsScrollLeft() {
	if c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].ScrollLeft()
	}
}

func (c *Content) ResultsScrollRight() {
	if c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].ScrollRight()
	}
}

func (c *Content) Update(msg tea.Msg) tea.Cmd {
	if len(c.textareas) == 0 || c.Tabs.Active >= len(c.textareas) {
		return nil
	}
	var cmd tea.Cmd
	c.textareas[c.Tabs.Active], cmd = c.textareas[c.Tabs.Active].Update(msg)
	return cmd
}

func (c *Content) ClearActive() {
	if len(c.textareas) > 0 && c.Tabs.Active < len(c.textareas) {
		c.textareas[c.Tabs.Active].Reset()
	}
}

func (c *Content) SetActiveText(text string) {
	if len(c.textareas) > 0 && c.Tabs.Active < len(c.textareas) {
		c.textareas[c.Tabs.Active].SetValue(text)
	}
}

func (c Content) CurrentRow() (headers []string, values []string, ok bool) {
	if c.Tabs.Active < len(c.results) {
		return c.results[c.Tabs.Active].CurrentRow()
	}
	return nil, nil, false
}

func (c Content) HasResults() bool {
	if c.Tabs.Active < len(c.results) {
		return c.results[c.Tabs.Active].HasData()
	}
	return false
}

func (c Content) ActiveText() string {
	if len(c.textareas) > 0 && c.Tabs.Active < len(c.textareas) {
		return c.textareas[c.Tabs.Active].Value()
	}
	return ""
}

func (c Content) ActiveResultData() ResultData {
	if len(c.results) > 0 && c.Tabs.Active < len(c.results) {
		return c.results[c.Tabs.Active].data
	}
	return ResultData{}
}

func (c *Content) SetResult(data ResultData) {
	if len(c.results) > 0 && c.Tabs.Active < len(c.results) {
		c.results[c.Tabs.Active].SetData(data)
	}
}

func (c *Content) CloseTab() {
	if len(c.Tabs.Items) <= 1 {
		return
	}
	idx := c.Tabs.Active
	c.Tabs.Items = append(c.Tabs.Items[:idx], c.Tabs.Items[idx+1:]...)
	c.textareas = append(c.textareas[:idx], c.textareas[idx+1:]...)
	c.results = append(c.results[:idx], c.results[idx+1:]...)

	if c.Tabs.Active >= len(c.Tabs.Items) {
		c.Tabs.Active = len(c.Tabs.Items) - 1
	}

	for i := range c.Tabs.Items {
		c.Tabs.Items[i].Title = fmt.Sprintf("%d", i+1)
	}

	focused := c.Tabs.Focused
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
	if focused {
		c.textareas[c.Tabs.Active].Focus()
	}
	c.Tabs.Focused = focused
}

func (c *Content) NextTab() {
	if len(c.Tabs.Items) == 0 {
		return
	}
	focused := c.Tabs.Focused
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
	c.Tabs.Active = (c.Tabs.Active + 1) % len(c.Tabs.Items)
	if focused {
		c.textareas[c.Tabs.Active].Focus()
	}
	c.Tabs.Focused = focused
}

func (c *Content) GoToTab(n int) {
	if n < 0 || n >= len(c.Tabs.Items) {
		return
	}
	focused := c.Tabs.Focused
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
	c.Tabs.Active = n
	if focused {
		c.textareas[c.Tabs.Active].Focus()
	}
	c.Tabs.Focused = focused
}

func (c *Content) PrevTab() {
	if len(c.Tabs.Items) == 0 {
		return
	}
	focused := c.Tabs.Focused
	for i := range c.textareas {
		c.textareas[i].Blur()
	}
	c.Tabs.Active = (c.Tabs.Active - 1 + len(c.Tabs.Items)) % len(c.Tabs.Items)
	if focused {
		c.textareas[c.Tabs.Active].Focus()
	}
	c.Tabs.Focused = focused
}

func (c Content) View(width, height int) string {
	header := c.Tabs.View(width)

	bodyHeight := max(0, height-1)
	topHeight := bodyHeight / 2
	bottomHeight := bodyHeight - topHeight

	var editor string
	if len(c.textareas) > 0 && c.Tabs.Active < len(c.textareas) {
		editor = highlightSQL(c.textareas[c.Tabs.Active].View(), c.theme)
	}

	editorStyle := lipgloss.NewStyle().
		Background(c.theme.Bg).
		Width(width).
		Height(topHeight)

	var resultsView string
	if len(c.results) > 0 && c.Tabs.Active < len(c.results) {
		resultsView = c.results[c.Tabs.Active].View(width, bottomHeight, c.resultsFocused)
	} else {
		resultsView = lipgloss.NewStyle().
			Background(c.theme.Bg).
			Width(width).
			Height(bottomHeight).
			Render("")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, editorStyle.Render(editor), resultsView)
}
