package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type TextInputOpt interface {
	Apply(*inputModel)
}

func newInput(opts ...TextInputOpt) (string, error) {
	ti := textinput.New()
	ti.Focus()
	mdl := inputModel{
		textInput: ti,
		err:       nil,
	}

	for _, opt := range opts {
		opt.Apply(&mdl)
	}

	p := tea.NewProgram(mdl)
	m, err := p.Run()
	if err != nil {
		return "", err
	}

	return m.(inputModel).textInput.Value(), nil
}

type (
	errMsg error
)

type inputModel struct {
	textInput textinput.Model
	err       error
}

func (m inputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

	// We handle errors just like any other message
	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m inputModel) View() string {
	return fmt.Sprintf("%s\n\n", m.textInput.View())
}

var _ TextInputOpt = (*textInputOptPlaceholder)(nil)

type textInputOptPlaceholder struct {
	val string
}

func (t textInputOptPlaceholder) Apply(s *inputModel) {
	s.textInput.Placeholder = t.val
}

func TextInputOptPlaceholder(placeholder string) TextInputOpt {
	return textInputOptPlaceholder{placeholder}
}

var _ TextInputOpt = (*textInputOptValue)(nil)

type textInputOptValue struct {
	val string
}

func (t textInputOptValue) Apply(s *inputModel) {
	s.textInput.SetValue(t.val)
}

func TextInputOptValue(val string) TextInputOpt {
	return textInputOptValue{val}
}
