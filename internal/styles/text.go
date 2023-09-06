package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	D8XPurple = lipgloss.Color("#664adf")
)

// Texts
var (
	PurpleBgText = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(D8XPurple)

	ErrorText = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#9a031e")).
			PaddingLeft(1).
			PaddingRight(1)

	SuccessText = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#09814a")).
			PaddingLeft(1).
			PaddingRight(1)

	ItalicText = lipgloss.NewStyle().
			Italic(true)

	CommandTitleText = ItalicText.Copy().Bold(true)
)

var (
	Button = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#fefefe")).
		Border(lipgloss.RoundedBorder()).
		Padding(0, 3, 0, 3)

	ButtonActive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#fefefe")).
			Border(lipgloss.RoundedBorder()).
			Background(D8XPurple).
			Underline(true).
			Padding(0, 3, 0, 3)
)

// Alerts, etc
var (
	// For warnings, passwords displays, etc
	AlertImportant = lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.Color("#d90429")).
		Foreground(lipgloss.Color("#fefefe")).
		Padding(1, 3, 1, 3)
)

// PrintCommandTitle is a helper to print (sub)command titles
func PrintCommandTitle(title string) {
	fmt.Printf(
		"%s\n\n",
		CommandTitleText.Render(title),
	)
}
