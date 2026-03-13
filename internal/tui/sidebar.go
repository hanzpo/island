package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// SidebarModel renders the left sidebar showing the workspace list.
type SidebarModel struct {
	keys MainKeyMap
}

func newSidebarModel() SidebarModel {
	return SidebarModel{
		keys: defaultMainKeys(),
	}
}

// View renders the sidebar panel with a list of workspaces.
// The selected workspace is highlighted. Width and height define the
// available space (not including the right border character).
func (s *SidebarModel) View(workspaces []*agent.Workspace, selected int, width, height, spinnerFrame int) string {
	var b strings.Builder

	// Header.
	header := sidebarHeaderStyle.Render("WORKSPACES")
	b.WriteString(header)
	b.WriteByte('\n')

	// Content width available for name + icon (minus padding/border overhead).
	nameWidth := width - 5 // 2 prefix ("▸ " or "  ") + 1 space + 1 icon + 1 padding

	if nameWidth < 4 {
		nameWidth = 4
	}

	linesUsed := 2 // header + blank line after header (from padding)

	for i, ws := range workspaces {
		if linesUsed >= height {
			break
		}

		name := ws.Name
		if name == "" {
			name = ws.ID
		}
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "\u2026"
		}

		icon := statusIconFor(ws.Status(), spinnerFrame)

		var line string
		if i == selected {
			label := "\u25b8 " + name
			// Pad to fill width so the highlight background covers the line.
			padded := label + strings.Repeat(" ", max(0, width-3-lipgloss.Width(label)-lipgloss.Width(icon))) + icon
			line = sidebarActiveStyle.Render(padded)
		} else {
			label := "  " + name
			padded := label + strings.Repeat(" ", max(0, width-3-lipgloss.Width(label)-lipgloss.Width(icon))) + icon
			line = sidebarItemStyle.Render(padded)
		}

		b.WriteString(line)
		linesUsed++
		if linesUsed < height {
			b.WriteByte('\n')
		}
	}

	// Fill remaining height with empty lines.
	for linesUsed < height {
		b.WriteByte('\n')
		linesUsed++
	}

	content := b.String()

	// Apply the sidebar style which adds a right border.
	return sidebarStyle.
		Width(width - 2). // subtract border width
		Height(height).
		Render(content)
}

// statusIconFor returns a styled status icon for a workspace status.
// The frame parameter drives the spinner animation for active states.
func statusIconFor(s agent.WorkspaceStatus, frame int) string {
	switch s {
	case agent.StatusInitializing:
		f := spinnerFrames[frame%len(spinnerFrames)]
		return waitingStyle.Render(f)
	case agent.StatusRunning:
		f := spinnerFrames[frame%len(spinnerFrames)]
		return runningStyle.Render(f)
	case agent.StatusWaiting:
		return waitingStyle.Render("\u25cb")
	case agent.StatusCompleted:
		return completedStyle.Render("\u2713")
	case agent.StatusErrored:
		return erroredStyle.Render("\u2717")
	case agent.StatusCancelled:
		return lipgloss.NewStyle().Faint(true).Render("\u25cc")
	case agent.StatusMerging:
		return runningStyle.Render("\u27f3")
	default:
		return "?"
	}
}
