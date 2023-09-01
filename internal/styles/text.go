package styles

import "github.com/charmbracelet/lipgloss"

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
)
