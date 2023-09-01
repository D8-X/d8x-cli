package components

import (
	"fmt"
	"strings"

	"github.com/D8-X/d8x-cli/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectionOpts modifies selectionModel instance
type SelectionOpts interface {
	Apply(*selectionModel)
}

// NewSelection runs a selection component and returns selected elements on
// success. If SelectionOptAllowOnlySingleItem option is passed, returned slice
// will always contain up to 1 item.
func NewSelection(selection []string, opts ...SelectionOpts) ([]string, error) {
	s := selectionModel{
		selection: selection,
		selected:  make([]bool, len(selection)),
	}

	for _, opt := range opts {
		opt.Apply(&s)
	}

	out, err := tea.NewProgram(s).Run()

	if err != nil {
		return nil, err
	}

	selected := make([]string, 0, len(selection))
	result := out.(selectionModel)

	for i := range result.selected {
		if result.selected[i] {
			selected = append(selected, result.selection[i])
		}
	}

	return selected, nil
}

var _ tea.Model = (*selectionModel)(nil)

type selectionModel struct {
	selection    []string
	selected     []bool
	cursor       int
	doneSelected bool

	oneSelectedOnly bool
}

func (m selectionModel) View() string {
	b := strings.Builder{}

	// Create: > [x] item line
	for i := range m.selection {
		arrow := "  "
		if m.cursor == i {
			arrow = "> "
		}
		selected := " "
		if m.selected[i] {
			selected = "x"
		}

		fmt.Fprintf(&b, "%s [%s] %s\n", arrow, selected, m.selection[i])
	}

	okButton := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fefefe")).
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1, 0, 1).
		Margin(1)
	if m.cursor == len(m.selection) {
		okButton.Background(styles.D8XPurple)
	}
	fmt.Fprintf(&b, "%s\n", okButton.Render("OK"))

	return lipgloss.NewStyle().Render(b.String())
}

func (m selectionModel) Init() tea.Cmd {
	return nil
}

func (m selectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.selection) {
				m.cursor++
			}
		case "enter", " ":
			if m.cursor < len(m.selection) {
				// When only 1 item can be selected, others must be cleared
				if m.oneSelectedOnly {
					for i := range m.selected {
						// Except current cursor
						if i != m.cursor {
							m.selected[i] = false
						}
					}
				}
				m.selected[m.cursor] = !m.selected[m.cursor]

			} else {
				// When done is clicked, exit
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

var _ SelectionOpts = (*selectionOptAllowOnlySingleItem)(nil)

type selectionOptAllowOnlySingleItem struct{}

func (selectionOptAllowOnlySingleItem) Apply(s *selectionModel) {
	s.oneSelectedOnly = true
}

func SelectionOptAllowOnlySingleItem() SelectionOpts {
	return selectionOptAllowOnlySingleItem{}
}
