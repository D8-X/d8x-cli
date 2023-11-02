package components

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newPrompt(question string, confirmed bool) (bool, error) {
	p := tea.NewProgram(promptModel{
		question:  question,
		confirmed: confirmed,
	})
	out, err := p.Run()

	if err != nil {
		return false, err
	}

	if v, ok := out.(exitModel); ok {
		return false, fmt.Errorf(v.Message())
	}

	result := out.(promptModel)

	return result.confirmed, nil
}

var _ tea.Model = (*promptModel)(nil)

type promptModel struct {
	question  string
	confirmed bool
}

func (m promptModel) View() string {
	yes, no := styles.Button, styles.Button
	if m.confirmed {
		yes = styles.ButtonActive
	} else {
		no = styles.ButtonActive
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n",
		m.question,
		lipgloss.JoinHorizontal(lipgloss.Left,
			yes.Copy().MarginRight(2).Render("yes"),
			no.Render("no"),
		),
	)
}

func (m promptModel) Init() tea.Cmd {
	return nil
}

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return exitModel{}, tea.Quit
		case "q", "enter":
			return m, tea.Quit
		case "right", "left":
			m.confirmed = !m.confirmed
		}
	}
	return m, nil
}
