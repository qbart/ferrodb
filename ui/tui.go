package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/qbart/ferrodb/ferro/plugin"
	"github.com/qbart/ferrodb/plugins"
)

type FocusArea int

const (
	FocusEditor FocusArea = iota
	FocusTree
	FocusResults
)

type queryDoneMsg struct {
	data ResultData
	ms   int64
}

type loadDataMsg struct {
	items []TreeItem
	err   error
}

type itemLoadedMsg struct {
	ids      []string
	children []TreeItem
}

type showItemMsg struct {
	query   string
	autoRun bool
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
	rowDetail   RowDetail
	explainView ExplainView
	footer      Footer
}

func New(opts Options) TUI {
	theme := DefaultTheme

	sidebar := NewSidebar(theme)
	sidebar.Tree.Items = []TreeItem{}
	if opts.Version != "" {
		sidebar.Title = "ferroDB"
		sidebar.Version = opts.Version
	}

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
		rowDetail:   NewRowDetail(theme),
		explainView: NewExplainView(theme),
		footer:      NewFooter(theme),
	}
}

func (t *TUI) sidebarTreeHeight() int {
	return max(0, t.height-4)
}

func (t *TUI) resultsDataHeight() int {
	mainHeight := t.height - 1
	bodyHeight := max(0, mainHeight-1)
	topHeight := bodyHeight / 2
	return max(0, bodyHeight-topHeight-1) // -1 for column header row
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

func errResult(msg string) ResultData {
	return ResultData{Headers: []string{"error"}, Rows: [][]string{{msg}}}
}

func runQueryCmd(opts Options, sql string, start time.Time) tea.Cmd {
	return func() tea.Msg {
		browser, err := opts.Registry.GetBrowser(opts.RawDriver)
		if err != nil {
			return queryDoneMsg{data: errResult(err.Error()), ms: time.Since(start).Milliseconds()}
		}
		ctx := context.Background()
		if err := browser.Connect(ctx, opts.RawDSN); err != nil {
			return queryDoneMsg{data: errResult(err.Error()), ms: time.Since(start).Milliseconds()}
		}
		defer browser.Disconnect(ctx)
		result, err := browser.Query(ctx, sql)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			return queryDoneMsg{data: errResult(err.Error()), ms: elapsed}
		}
		return queryDoneMsg{data: ResultData{Headers: result.Headers, Rows: result.Rows, ColumnTypes: result.ColumnTypes}, ms: elapsed}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func showItemCmd(opts Options, ids []string, autoRun bool) tea.Cmd {
	return func() tea.Msg {
		browser, err := opts.Registry.GetBrowser(opts.RawDriver)
		if err != nil {
			return nil
		}
		ctx := context.Background()
		if err := browser.Connect(ctx, opts.RawDSN); err != nil {
			return nil
		}
		defer browser.Disconnect(ctx)
		query, err := browser.Show(ctx, ids)
		if err != nil || query == "" {
			return nil
		}
		return showItemMsg{query: query, autoRun: autoRun}
	}
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
		t.content.SetResult(msg.data)
		return t, nil

	case loadDataMsg:
		if msg.err == nil {
			t.sidebar.Tree.Items = msg.items
			t.sidebar.Tree.Cursor = 0
		}
		return t, nil

	case itemLoadedMsg:
		t.sidebar.Tree.SetLoaded(msg.ids, msg.children)
		t.sidebar.Tree.EnsureVisible(t.sidebarTreeHeight())
		return t, nil

	case showItemMsg:
		t.content.AddTab()
		t.content.SetActiveText(msg.query)
		t.focus = FocusEditor
		t.content.Focus()
		t.sidebar.Tree.Focused = false
		if msg.autoRun && msg.query != "" && !t.footer.Running {
			now := time.Now()
			t.footer.Running = true
			t.footer.QueryDone = false
			t.footer.QueryStart = now
			return t, tea.Batch(runQueryCmd(t.opts, msg.query, now), tickCmd())
		}
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
		// overlays consume all keys first
		if t.help.Visible {
			switch msg.String() {
			case "f1", "esc", "ctrl+c":
				t.help.Visible = false
			}
			return t, nil
		}

		if t.rowDetail.Visible {
			switch msg.String() {
			case "esc", "enter", "ctrl+c":
				t.rowDetail.Close()
			case "up":
				t.rowDetail.ScrollUp()
			case "down":
				all := t.rowDetail.buildLines()
				t.rowDetail.ScrollDown(len(all), t.height-1)
			case "j":
				t.rowDetail.JumpNext()
			case "k":
				t.rowDetail.JumpPrev()
			case "left":
				t.rowDetail.ScrollLeft()
			case "right":
				t.rowDetail.ScrollRight()
			case "y":
				t.rowDetail.Copy()
			}
			return t, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return t, tea.Quit
		case "tab":
			switch t.focus {
			case FocusTree:
				t.focus = FocusEditor
				t.sidebar.Tree.Focused = false
				t.content.Focus()
			case FocusEditor:
				if t.content.HasResults() {
					t.focus = FocusResults
					t.content.Blur()
					t.content.FocusResults()
				} else if t.sidebarOpen {
					t.focus = FocusTree
					t.content.Blur()
					t.sidebar.Tree.Focused = true
				}
			case FocusResults:
				if t.sidebarOpen {
					t.focus = FocusTree
					t.content.BlurResults()
					t.sidebar.Tree.Focused = true
				} else {
					t.focus = FocusEditor
					t.content.BlurResults()
					t.content.Focus()
				}
			}
		case "shift+tab":
			switch t.focus {
			case FocusTree:
				if t.content.HasResults() {
					t.focus = FocusResults
					t.sidebar.Tree.Focused = false
					t.content.FocusResults()
				} else {
					t.focus = FocusEditor
					t.sidebar.Tree.Focused = false
					t.content.Focus()
				}
			case FocusEditor:
				if t.sidebarOpen {
					t.focus = FocusTree
					t.content.Blur()
					t.sidebar.Tree.Focused = true
				} else if t.content.HasResults() {
					t.focus = FocusResults
					t.content.Blur()
					t.content.FocusResults()
				}
			case FocusResults:
				t.focus = FocusEditor
				t.content.BlurResults()
				t.content.Focus()
			}
			return t, nil
		case "f1":
			t.help.Visible = true
			return t, nil
		case "ctrl+w":
			if t.focus == FocusEditor {
				t.content.CloseTab()
			}
			return t, nil
		case "ctrl+g":
			t.navbar.Next()
			return t, nil
		case "ctrl+t":
			t.content.AddTab()
			return t, nil
		case "ctrl+right", "ctrl+l":
			t.content.NextTab()
			return t, nil
		case "ctrl+left", "ctrl+h":
			t.content.PrevTab()
			return t, nil
		case "ctrl+\\":
			t.sidebarOpen = !t.sidebarOpen
			if !t.sidebarOpen && t.focus == FocusTree {
				t.focus = FocusEditor
				t.sidebar.Tree.Focused = false
				t.content.Focus()
			}
			t.resizeContent()
			return t, nil
		case "ctrl+r":
			if t.footer.Running {
				return t, nil
			}
			sql := t.content.ActiveText()
			if sql == "" {
				return t, nil
			}
			now := time.Now()
			t.footer.Running = true
			t.footer.QueryDone = false
			t.footer.QueryStart = now
			return t, tea.Batch(runQueryCmd(t.opts, sql, now), tickCmd())
		case "ctrl+e":
			data := t.content.ActiveResultData()
			browser, err := t.opts.Registry.GetBrowser(t.opts.RawDriver)
			if err != nil {
				t.explainView.SetError(err.Error())
				t.navbar.Active = NavExplain
				return t, nil
			}
			result, err := browser.ParseExplain(plugin.BrowserQueryResult{
				Headers:     data.Headers,
				Rows:        data.Rows,
				ColumnTypes: data.ColumnTypes,
			})
			if err != nil {
				t.explainView.SetError(err.Error())
			} else {
				t.explainView.SetResult(result)
			}
			t.navbar.Active = NavExplain
			return t, nil
		}

		if t.navbar.Active == NavExplain {
			switch msg.String() {
			case "up", "k":
				t.explainView.ScrollUp()
			case "down", "j":
				t.explainView.ScrollDown(t.height-2, t.width-navWidth-1)
			}
			return t, nil
		}

		if t.focus == FocusResults {
			switch msg.String() {
			case "up", "k":
				t.content.ResultsMoveUp()
				t.content.ResultsEnsureVisible(t.resultsDataHeight())
			case "down", "j":
				t.content.ResultsMoveDown()
				t.content.ResultsEnsureVisible(t.resultsDataHeight())
			case "left", "h":
				t.content.ResultsScrollLeft()
			case "right", "l":
				t.content.ResultsScrollRight()
			case "enter":
				if headers, values, ok := t.content.CurrentRow(); ok {
					t.rowDetail.Open(headers, values)
				}
			}
			return t, nil
		}

		if t.focus == FocusTree {
			switch msg.String() {
			case "up", "k":
				t.sidebar.Tree.MoveUp()
				t.sidebar.Tree.EnsureVisible(t.sidebarTreeHeight())
			case "down", "j":
				t.sidebar.Tree.MoveDown()
				t.sidebar.Tree.EnsureVisible(t.sidebarTreeHeight())
			case "left", "h":
				t.sidebar.Tree.Collapse()
				t.sidebar.Tree.EnsureVisible(t.sidebarTreeHeight())
			case "right", "l":
				if ids, ok := t.sidebar.Tree.StartLoading(); ok {
					return t, tea.Batch(loadItemCmd(t.opts, ids), tickCmd())
				}
				t.sidebar.Tree.Expand()
				t.sidebar.Tree.EnsureVisible(t.sidebarTreeHeight())
			case "enter":
				return t, showItemCmd(t.opts, t.sidebar.Tree.CursorIDPath(), true)
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
	if t.navbar.Active == NavExplain {
		sepWidth := 1
		explainWidth := t.width - navWidth - sepWidth
		nav := t.navbar.View(mainHeight)
		header := lipgloss.NewStyle().
			Width(explainWidth+sepWidth).
			Background(t.theme.SidebarHeaderBg).
			Foreground(t.theme.SidebarHeaderFg).
			Bold(true).
			Render(" Explain")
		sepLine := lipgloss.NewStyle().
			Background(t.theme.NavBg).
			Foreground(t.theme.SidebarHeaderBg).
			Render("▏")
		bodyHeight := mainHeight - 1
		sep := strings.Repeat(sepLine+"\n", bodyHeight-1) + sepLine
		body := t.explainView.View(explainWidth, bodyHeight)
		bodyRow := lipgloss.JoinHorizontal(lipgloss.Top, sep, body)
		explain := lipgloss.JoinVertical(lipgloss.Left, header, bodyRow)
		main = lipgloss.JoinHorizontal(lipgloss.Top, nav, explain)
	} else if t.sidebarOpen {
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

	footerLabel := ""
	if t.focus == FocusTree {
		if labels, ok := t.sidebar.Tree.CursorPath(); ok {
			depth := len(labels) - 1
			switch depth {
			case 1:
				footerLabel = labels[0] // "Tables"/"Views" level: show schema name
			case 3:
				footerLabel = labels[2] // sub-category level (Columns/Indexes/…): show table name
			default:
				footerLabel = labels[depth]
			}
		}
	}
	footer := t.footer.View(t.width, footerLabel)

	screen := main + "\n" + footer

	if t.help.Visible {
		return t.help.View(t.width, t.height)
	}

	if t.rowDetail.Visible {
		return t.rowDetail.View(t.width, t.height)
	}

	return screen
}

type Options struct {
	RawDriver string
	RawDSN    string
	Registry  *plugins.Registry
	Version   string
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
