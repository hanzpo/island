package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// SidebarModel renders the workspace list panel.
type SidebarModel struct {
	scroll int // first visible index
}

func newSidebarModel() SidebarModel {
	return SidebarModel{}
}

// EnsureVisible adjusts scroll so the selected index is visible within the given height.
func (s *SidebarModel) EnsureVisible(selected, visibleRows int) {
	if visibleRows <= 0 {
		return
	}
	if selected < s.scroll {
		s.scroll = selected
	}
	if selected >= s.scroll+visibleRows {
		s.scroll = selected - visibleRows + 1
	}
}

// View renders the sidebar panel content (without border — border is applied by app).
func (s *SidebarModel) View(workspaces []*agent.Workspace, selected int, width, height, spinnerFrame int) string {
	if height <= 0 || width <= 0 {
		return ""
	}

	s.EnsureVisible(selected, height)

	var b strings.Builder

	// Name width: total - prefix (2) - gap (1) - icon (2) - safety (1)
	nameWidth := width - 6
	if nameWidth < 4 {
		nameWidth = 4
	}

	linesUsed := 0
	end := s.scroll + height
	if end > len(workspaces) {
		end = len(workspaces)
	}

	for i := s.scroll; i < end; i++ {
		if linesUsed > 0 {
			b.WriteByte('\n')
		}

		ws := workspaces[i]
		name := ws.Name
		if name == "" {
			name = ws.ID
		}
		if len(name) > nameWidth {
			name = name[:nameWidth-1] + "\u2026"
		}

		icon := statusIconChar(ws.Status(), spinnerFrame)
		iconStyled := statusIconStyled(ws.Status(), spinnerFrame)

		var line string
		if i == selected {
			// Selected: highlighted prefix + bold name + icon
			prefix := lipgloss.NewStyle().Foreground(colorBorderActive).Bold(true).Render("\u25b8 ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(colorBright).Render(name)
			pad := width - 2 - lipgloss.Width(name) - lipgloss.Width(icon)
			if pad < 1 {
				pad = 1
			}
			line = prefix + nameStyled + strings.Repeat(" ", pad) + iconStyled
		} else {
			prefix := "  "
			nameStyled := styleSubtle.Render(name)
			if ws.Archived {
				nameStyled = styleArchived.Render(name)
			}
			pad := width - 2 - lipgloss.Width(name) - lipgloss.Width(icon)
			if pad < 1 {
				pad = 1
			}
			line = prefix + nameStyled + strings.Repeat(" ", pad) + iconStyled
		}

		b.WriteString(zones.Mark(sidebarZoneID(i), line))
		linesUsed++
	}

	// Fill remaining height.
	for linesUsed < height {
		if linesUsed > 0 {
			b.WriteByte('\n')
		}
		linesUsed++
	}

	return b.String()
}

// statusIconChar returns the raw unicode character for a workspace status.
func statusIconChar(s agent.WorkspaceStatus, frame int) string {
	switch s {
	case agent.StatusInitializing, agent.StatusRunning:
		return spinnerFrames[frame%len(spinnerFrames)]
	case agent.StatusWaiting:
		return "\u25cb" // ○
	case agent.StatusCompleted, agent.StatusArchived:
		return "\u2713" // ✓
	case agent.StatusErrored:
		return "\u2717" // ✗
	case agent.StatusCancelled:
		return "\u25cc" // ◌
	case agent.StatusMerging:
		return "\u27f3" // ⟳
	case agent.StatusInReview:
		return "\u25c9" // ◉
	default:
		return "?"
	}
}

// statusIconStyled returns a styled status icon.
func statusIconStyled(s agent.WorkspaceStatus, frame int) string {
	icon := statusIconChar(s, frame)
	switch s {
	case agent.StatusInitializing:
		return styleWaiting.Render(icon)
	case agent.StatusRunning:
		return styleRunning.Render(icon)
	case agent.StatusWaiting:
		return styleWaiting.Render(icon)
	case agent.StatusCompleted:
		return styleCompleted.Render(icon)
	case agent.StatusErrored:
		return styleErrored.Render(icon)
	case agent.StatusCancelled:
		return styleCancelled.Render(icon)
	case agent.StatusMerging:
		return styleRunning.Render(icon)
	case agent.StatusInReview:
		return styleInReview.Render(icon)
	case agent.StatusArchived:
		return styleArchived.Render(icon)
	default:
		return icon
	}
}
