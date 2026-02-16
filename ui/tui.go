package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type TUI struct{}

func New() TUI {
	return TUI{}
}

func (t TUI) Init() tea.Cmd {
	return nil
}

func (t TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return t, tea.Quit
		}
	}
	return t, nil
}

func (t TUI) View() string {
	return fmt.Sprintf("ferroDB\n\nPress q to quit.\n")
}

func Run() error {
	p := tea.NewProgram(New())
	_, err := p.Run()
	return err
}
