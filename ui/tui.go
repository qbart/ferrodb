package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type queryDoneMsg struct {
	result string
	ms     int64
}

type tickMsg time.Time

type TUI struct {
	width       int
	height      int
	theme       Theme
	sidebarOpen bool
	navbar      Navbar
	sidebar     Sidebar
	content Content
	help    Help
	footer  Footer
}

func New() TUI {
	theme := DefaultTheme
	sidebar := NewSidebar(theme)
	sidebar.Tree.Items = []TreeItem{
		{Label: "public", Expanded: true, Children: []TreeItem{
			{Label: "users"},
			{Label: "orders"},
			{Label: "products"},
		}},
		{Label: "auth", Expanded: false, Children: []TreeItem{
			{Label: "sessions"},
			{Label: "tokens"},
		}},
		{Label: "analytics", Expanded: false, Children: []TreeItem{
			{Label: "events"},
			{Label: "pageviews"},
		}},
	}

	content := NewContent(theme)
	content.SetTabs([]TabItem{
		{Title: "1", Modified: false, Content: "SELECT u.id, u.name, u.email\nFROM users u\nJOIN orders o ON o.user_id = u.id\nWHERE o.created_at > NOW() - INTERVAL '30 days'\nGROUP BY u.id\nHAVING COUNT(o.id) > 5\nORDER BY u.name;"},
		{Title: "2", Content: "INSERT INTO products (name, price, category)\nVALUES\n  ('Widget A', 29.99, 'electronics'),\n  ('Widget B', 49.99, 'electronics'),\n  ('Gadget C', 99.99, 'accessories')\nRETURNING id, name;"},
		{Title: "3", Content: "UPDATE orders\nSET status = 'shipped',\n    shipped_at = NOW()\nWHERE status = 'processing'\n  AND created_at < NOW() - INTERVAL '2 days'\nRETURNING id, status;"},
	})
	content.Focus()

	return TUI{
		theme:   theme,
		navbar:  NewNavbar(theme),
		sidebar: sidebar,
		content: content,
		sidebarOpen: true,
		help:        NewHelp(theme),
		footer:      NewFooter(theme),
	}
}

func (t *TUI) resizeContent() {
	mainHeight := t.height - 1
	if t.sidebarOpen {
		sidebarWidth := t.width*25/100 - navWidth
		t.content.Resize(t.width-sidebarWidth-navWidth, mainHeight)
	} else {
		t.content.Resize(t.width-navWidth, mainHeight)
	}
}

func (t TUI) Init() tea.Cmd {
	return nil
}

func runFakeQuery(start time.Time) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)
		elapsed := time.Since(start).Milliseconds()
		return queryDoneMsg{
			result: " id | name       | email\n  1 | Alice      | alice@example.com\n  2 | Bob        | bob@example.com\n  3 | Charlie    | charlie@example.com\n  4 | Diana      | diana@example.com\n  5 | Eve        | eve@example.com\n\n(5 rows)",
			ms:     elapsed,
		}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (t TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		t.resizeContent()
		return t, nil

	case queryDoneMsg:
		t.footer.Running = false
		t.footer.QueryDone = true
		t.footer.QueryMs = msg.ms
		t.content.SetResult(msg.result)
		return t, nil

	case tickMsg:
		if t.footer.Running {
			t.footer.Tick()
			return t, tickCmd()
		}
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return t, tea.Quit
		case "f1":
			t.help.Visible = !t.help.Visible
			return t, nil
		case "ctrl+t":
			t.content.AddTab()
			return t, nil
		case "tab":
			t.content.NextTab()
			return t, nil
		case "shift+tab":
			t.content.PrevTab()
			return t, nil
		case "ctrl+\\":
			t.sidebarOpen = !t.sidebarOpen
			t.resizeContent()
			return t, nil
		case "ctrl+r":
			if t.footer.Running {
				return t, nil
			}
			now := time.Now()
			t.footer.Running = true
			t.footer.QueryDone = false
			t.footer.QueryStart = now
			return t, tea.Batch(runFakeQuery(now), tickCmd())
		}
	}

	cmd := t.content.Update(msg)
	return t, cmd
}

func (t TUI) View() string {
	if t.height == 0 || t.width == 0 {
		return ""
	}

	mainHeight := t.height - 1

	var main string
	if t.sidebarOpen {
		sidebarWidth := t.width*25/100 - navWidth
		contentWidth := t.width - sidebarWidth - navWidth
		nav := t.navbar.View(mainHeight)
		tree := t.sidebar.View(sidebarWidth, mainHeight)
		content := t.content.View(contentWidth, mainHeight)
		main = lipgloss.JoinHorizontal(lipgloss.Top, nav, tree, content)
	} else {
		contentWidth := t.width - navWidth
		nav := t.navbar.View(mainHeight)
		content := t.content.View(contentWidth, mainHeight)
		main = lipgloss.JoinHorizontal(lipgloss.Top, nav, content)
	}

	footer := t.footer.View(t.width)

	screen := main + "\n" + footer

	if t.help.Visible {
		return t.help.View(t.width, t.height)
	}

	return screen
}

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
