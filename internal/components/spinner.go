package components

// A simple program demonstrating the spinner component from the Bubbles
// component library.

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	spinner  spinner.Model
	quitting bool
	err      error

	text string
	done chan struct{}
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(styles.D8XPurple)
	return model{spinner: s}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return exitModel{}, tea.Quit
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		default:
			return m, nil
		}
	default:
		// Check if we should quit
		select {
		case <-m.done:
			return m, tea.Quit
		default:
		}

		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m model) View() string {
	return m.spinner.View() + " " + m.text
}

func newSpinner(done chan struct{}, text string) error {
	m := initialModel()
	m.text = text
	m.done = done

	p := tea.NewProgram(m)
	out, err := p.Run()

	if v, ok := out.(exitModel); ok {
		return fmt.Errorf(v.Message())
	}

	return err
}
