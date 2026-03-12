package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("62")  // muted blue
	secondaryColor = lipgloss.Color("241") // gray
	accentColor    = lipgloss.Color("212") // pink
	successColor   = lipgloss.Color("78")  // green
	errorColor     = lipgloss.Color("196") // red
	warnColor      = lipgloss.Color("214") // orange

	// Panel styles
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor).
			Padding(0, 1)

	activePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	// Header / footer
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	footerStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(secondaryColor)

	// Status indicators
	runningStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	completedStyle = lipgloss.NewStyle().
			Foreground(successColor)

	erroredStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	waitingStyle = lipgloss.NewStyle().
			Foreground(warnColor)

	// Diff colors
	diffAddedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78")) // green

	diffRemovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")) // red

	diffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("44")) // cyan

	diffHeaderStyle = lipgloss.NewStyle().
			Bold(true)

	diffContextStyle = lipgloss.NewStyle().
				Faint(true)

	// Dialog
	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(0, 0, 1, 0)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	// Workspace view
	turnSeparatorStyle = lipgloss.NewStyle().
				Faint(true)

	stderrStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("214")) // dim yellow

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(secondaryColor).
			Padding(0, 1)
)
