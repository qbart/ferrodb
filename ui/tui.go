package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TUI struct {
	width   int
	height  int
	theme   Theme
	navbar  Navbar
	sidebar Sidebar
	content Content
	footer  Footer
}

func New() TUI {
	theme := DefaultTheme
	return TUI{
		theme:   theme,
		navbar:  NewNavbar(theme),
		sidebar: NewSidebar(theme),
		content: NewContent(theme),
		footer:  NewFooter(theme),
	}
}

func (t TUI) Init() tea.Cmd {
	return nil
}

func (t TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return t, tea.Quit
		case "1":
			t.navbar.Active = NavDatabase
			return t, nil
		case "2":
			t.navbar.Active = NavFavourites
			return t, nil
		}
	}
	return t, nil
}

func (t TUI) View() string {
	if t.height == 0 || t.width == 0 {
		return ""
	}

	mainHeight := t.height - 1
	sidebarWidth := t.width*25/100 - navWidth
	contentWidth := t.width - sidebarWidth - navWidth

	nav := t.navbar.View(mainHeight)
	tree := t.sidebar.View(sidebarWidth, mainHeight)
	content := t.content.View(contentWidth, mainHeight)
	main := lipgloss.JoinHorizontal(lipgloss.Top, nav, tree, content)

	footer := t.footer.View(t.width)

	return main + "\n" + footer
}

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
