package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// HelpModel renders the help overlay.
type HelpModel struct {
	open bool
}

func newHelpModel() HelpModel {
	return HelpModel{}
}

// IsOpen returns whether the help overlay is visible.
func (h *HelpModel) IsOpen() bool {
	return h.open
}

// Toggle opens or closes the help overlay.
func (h *HelpModel) Toggle() {
	h.open = !h.open
}

// Close closes the help overlay.
func (h *HelpModel) Close() {
	h.open = false
}

// View renders the help overlay centered in the given dimensions.
func (h *HelpModel) View(width, height int) string {
	if !h.open {
		return ""
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitleActive).
		Render("Keyboard Shortcuts")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Navigation section
	b.WriteString(renderHelpSection("Navigation", GlobalHelp()))
	b.WriteString("\n")

	// Actions section
	b.WriteString(renderHelpSection("Actions", ActionHelp()))
	b.WriteString("\n")

	// Output section
	b.WriteString(renderHelpSection("Output", OutputHelp()))
	b.WriteString("\n")

	b.WriteString(styleSubtle.Render("Press ? or Esc to close"))

	content := b.String()

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorderActive).
		Padding(1, 3).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog)
}

func renderHelpSection(title string, bindings []HelpBinding) string {
	var b strings.Builder

	sectionTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBright).
		Render(title)
	b.WriteString(sectionTitle)
	b.WriteByte('\n')

	keyStyle := lipgloss.NewStyle().
		Foreground(colorBorderActive).
		Bold(true).
		Width(14)

	descStyle := lipgloss.NewStyle().
		Foreground(colorText)

	for _, bind := range bindings {
		b.WriteString("  ")
		b.WriteString(keyStyle.Render(bind.Key))
		b.WriteString(descStyle.Render(bind.Desc))
		b.WriteByte('\n')
	}

	return b.String()
}
