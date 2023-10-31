package components

import (
	"fmt"

	"github.com/D8-X/d8x-cli/internal/styles"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type ListItem struct {
	ItemTitle, ItemDesc string
}

func (i ListItem) Title() string       { return i.ItemTitle }
func (i ListItem) Description() string { return i.ItemDesc }
func (i ListItem) FilterValue() string { return i.ItemTitle }

type listModel struct {
	list list.Model
}

func (m listModel) Init() tea.Cmd {
	return nil
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return exitModel{}, tea.Quit
		}
		// Return value on enter or exit
		if msg.Type == tea.KeyEnter {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m listModel) View() string {
	return docStyle.Render(m.list.View())
}

func newList(listItems []ListItem, listTitle string, opts ...ListOpt) (ListItem, error) {
	items := make([]list.Item, len(listItems))

	for i, itm := range listItems {
		items[i] = itm
	}

	m := listModel{list: list.New(items, list.NewDefaultDelegate(), 0, 0)}
	m.list.Title = listTitle
	m.list.Styles.Title = styles.PurpleBgText.Copy().Padding(0, 1, 0, 1)

	// Apply opts after list is initialized
	for _, opt := range opts {
		opt(&m)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())

	mdl, err := p.Run()
	if err != nil {
		return ListItem{}, err
	}

	if v, ok := mdl.(exitModel); ok {
		return ListItem{}, fmt.Errorf(v.Message())
	}

	return mdl.(listModel).list.SelectedItem().(ListItem), nil
}

type ListOpt func(*listModel)

// Set the default selected item
func ListOptSelectedItem(item ListItem) ListOpt {
	return func(model *listModel) {
		for i, itm := range model.list.Items() {
			if itm == item {
				model.list.Select(i)
			}
		}
	}
}
