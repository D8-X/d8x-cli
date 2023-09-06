package components

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func NewConfirmation(text string) error {
	_, err := tea.NewProgram(confirmModel{
		text: text,
	}).Run()

	return err
}

var _ tea.Model = (*confirmModel)(nil)

type confirmModel struct {
	text string
}

func (m confirmModel) View() string {
	return fmt.Sprintf(
		"%s\n%s",
		m.text,
		styles.ItalicText.Copy().Faint(true).Render("[Enter to confirm]"),
	)
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}
