package components

import "time"

//go:generate mockgen -package mocks -destination ../mocks/components.go . ComponentsRunner

// ComponentsRunner exposes and runs the TUI components. Interface is used
// mainly as abstraction to allow mocking the components in tests without
// actually running them which would otherwise require complex testing logic to
// test all the IO interactions.
type ComponentsRunner interface {
	NewConfirmation(text string) error
	NewInput(opts ...TextInputOpt) (string, error)
	NewList(listItems []ListItem, listTitle string, opts ...ListOpt) (ListItem, error)
	NewPrompt(question string, confirmed bool) (bool, error)
	NewSelection(selection []string, opts ...SelectionOpts) ([]string, error)
	NewSpinner(done chan struct{}, text string) error
	NewTimer(timeout time.Duration, title string) error
}

var _ (ComponentsRunner) = (*InteractiveRunner)(nil)

// InteractiveRunner runs the actual TUI components
type InteractiveRunner struct{}

func (InteractiveRunner) NewConfirmation(text string) error {
	return newConfirmation(text)
}

func (InteractiveRunner) NewInput(opts ...TextInputOpt) (string, error) {
	return newInput(opts...)
}

func (InteractiveRunner) NewList(listItems []ListItem, listTitle string, opts ...ListOpt) (ListItem, error) {
	return newList(listItems, listTitle, opts...)
}

func (InteractiveRunner) NewPrompt(question string, confirmed bool) (bool, error) {
	return newPrompt(question, confirmed)
}

func (InteractiveRunner) NewSelection(selection []string, opts ...SelectionOpts) ([]string, error) {
	return newSelection(selection, opts...)
}

func (InteractiveRunner) NewSpinner(done chan struct{}, text string) error {
	return newSpinner(done, text)
}

func (InteractiveRunner) NewTimer(timeout time.Duration, title string) error {
	return newTimer(timeout, title)
}
