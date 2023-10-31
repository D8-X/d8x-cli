package components

import tea "github.com/charmbracelet/bubbletea"

var _ tea.Model = (*exitModel)(nil)

// exitModel is returned when TUI component is killed with ctrl-c command
type exitModel struct{}

func (e exitModel) Message() string {
	return "user interrupted program execution"
}

func (e exitModel) Init() tea.Cmd {
	return nil
}
func (e exitModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return nil, nil
}
func (e exitModel) View() string {
	return ""
}
