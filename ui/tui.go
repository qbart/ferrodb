package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qbart/ferrodb/plugins"
)

type FocusArea int

const (
	FocusEditor FocusArea = iota
	FocusTree
)

type queryDoneMsg struct {
	result string
	ms     int64
}

type loadDataMsg struct {
	items []TreeItem
	err   error
}

type itemLoadedMsg struct {
	ids      []string
	children []TreeItem
}

type tickMsg time.Time

type TUI struct {
	width       int
	height      int
	theme       Theme
	sidebarOpen bool
	focus       FocusArea
	opts        Options
	navbar      Navbar
	sidebar     Sidebar
	content     Content
	help        Help
	footer      Footer
}

func New(opts Options) TUI {
	theme := DefaultTheme

	sidebar := NewSidebar(theme)
	sidebar.Tree.Items = []TreeItem{}

	content := NewContent(theme)
	content.SetTabs([]TabItem{
		{Title: "1", Modified: false, Content: ""},
	})
	content.Focus()

	return TUI{
		theme:       theme,
		opts:        opts,
		focus:       FocusEditor,
		navbar:      NewNavbar(theme),
		sidebar:     sidebar,
		content:     content,
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

func (t *TUI) LoadData(ctx context.Context) error {
	browser, err := t.opts.Registry.GetBrowser(t.opts.RawDriver)
	if err != nil {
		return err
	}
	if err := browser.Connect(ctx, t.opts.RawDSN); err != nil {
		return err
	}
	defer browser.Disconnect(ctx)

	items, err := browser.List(ctx, []string{})
	if err != nil {
		return err
	}

	for _, item := range items {
		t.sidebar.Tree.Items = append(t.sidebar.Tree.Items, TreeItem{
			ID:         item.ID,
			Label:      item.Name,
			Expandable: item.HasChildren,
		})
	}

	return nil
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

func loadItemCmd(opts Options, ids []string) tea.Cmd {
	return func() tea.Msg {
		browser, err := opts.Registry.GetBrowser(opts.RawDriver)
		if err != nil {
			return itemLoadedMsg{ids: ids}
		}
		ctx := context.Background()
		if err := browser.Connect(ctx, opts.RawDSN); err != nil {
			return itemLoadedMsg{ids: ids}
		}
		defer browser.Disconnect(ctx)

		browserItems, err := browser.List(ctx, ids)
		if err != nil {
			return itemLoadedMsg{ids: ids}
		}

		children := make([]TreeItem, len(browserItems))
		for i, item := range browserItems {
			children[i] = TreeItem{
				ID:         item.ID,
				Label:      item.Name,
				Expandable: item.HasChildren,
			}
		}
		return itemLoadedMsg{ids: ids, children: children}
	}
}

func (t TUI) reloadCmd() tea.Cmd {
	opts := t.opts
	return func() tea.Msg {
		browser, err := opts.Registry.GetBrowser(opts.RawDriver)
		if err != nil {
			return loadDataMsg{err: err}
		}
		ctx := context.Background()
		if err := browser.Connect(ctx, opts.RawDSN); err != nil {
			return loadDataMsg{err: err}
		}
		defer browser.Disconnect(ctx)

		browserItems, err := browser.List(ctx, []string{})
		if err != nil {
			return loadDataMsg{err: err}
		}

		items := make([]TreeItem, len(browserItems))
		for i, item := range browserItems {
			items[i] = TreeItem{
				ID:         item.ID,
				Label:      item.Name,
				Expandable: item.HasChildren,
			}
		}
		return loadDataMsg{items: items}
	}
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

	case loadDataMsg:
		if msg.err == nil {
			t.sidebar.Tree.Items = msg.items
			t.sidebar.Tree.Cursor = 0
		}
		return t, nil

	case itemLoadedMsg:
		t.sidebar.Tree.SetLoaded(msg.ids, msg.children)
		return t, nil

	case tickMsg:
		treeLoading := t.sidebar.Tree.IsLoading()
		if t.footer.Running {
			t.footer.Tick()
		}
		if treeLoading {
			t.sidebar.Tree.AdvanceSpinner()
		}
		if t.footer.Running || treeLoading {
			return t, tickCmd()
		}
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return t, tea.Quit
		case "ctrl+w":
			if t.focus == FocusEditor {
				t.focus = FocusTree
				t.content.Blur()
				t.sidebar.Tree.Focused = true
			} else {
				t.focus = FocusEditor
				t.content.Focus()
				t.sidebar.Tree.Focused = false
			}
			return t, nil
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

		if t.focus == FocusTree {
			switch msg.String() {
			case "w":
				t.sidebar.Tree.MoveUp()
			case "s":
				t.sidebar.Tree.MoveDown()
			case "a":
				t.sidebar.Tree.Collapse()
			case "d":
				if ids, ok := t.sidebar.Tree.StartLoading(); ok {
					return t, tea.Batch(loadItemCmd(t.opts, ids), tickCmd())
				}
				t.sidebar.Tree.Expand()
			case "R":
				return t, t.reloadCmd()
			}
			return t, nil
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

type Options struct {
	RawDriver string
	RawDSN    string
	Registry  *plugins.Registry
}

func Run(ctx context.Context, opts Options) error {
	tui := New(opts)
	err := tui.LoadData(ctx)
	if err != nil {
		return err
	}
	p := tea.NewProgram(tui, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
