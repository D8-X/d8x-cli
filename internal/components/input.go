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

	mdl = m.(inputModel)
	returnValue := mdl.textInput.Value()
	if mdl.masked {
		returnValue = mdl.value
	}

	return returnValue, nil
}

type (
	errMsg error
)

type inputModel struct {
	textInput textinput.Model
	err       error

	// Whether the displayed value should be masked
	masked bool
	// The actual value of text input, if masked is true, the value in textInput
	// will be masked and this value will represent the actual value.
	value string
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

	// When input is masked, we store the value on inputModel, process a
	// keystroke via internal m.textInput and then mask it.
	if m.masked {
		m.textInput.SetValue(m.value)
		m.textInput, cmd = m.textInput.Update(msg)
		m.value = m.textInput.Value()

		// Mask it
		masked := ""
		for range m.value {
			masked += "*"
		}
		m.textInput.SetValue(masked)

	} else {
		m.textInput, cmd = m.textInput.Update(msg)
	}

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
	s.value = t.val
}

func TextInputOptValue(val string) TextInputOpt {
	return textInputOptValue{val}
}

var _ TextInputOpt = (*testInputOptMasked)(nil)

type testInputOptMasked struct {
	val string
}

func (t testInputOptMasked) Apply(s *inputModel) {
	s.masked = true
}

func TextInputOptMasked() TextInputOpt {
	return testInputOptMasked{}
}
