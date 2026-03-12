package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// DashboardModel represents the dashboard screen showing a grid of workspace panels.
type DashboardModel struct {
	selected int
	keys     DashboardKeyMap
}

func newDashboardModel() DashboardModel {
	return DashboardModel{
		selected: 0,
		keys:     defaultDashboardKeys(),
	}
}

// Update handles navigation on the dashboard. Key actions like Enter, n, d
// are handled in App.Update; this only handles cursor movement.
func (d *DashboardModel) Update(msg tea.Msg, workspaceCount int) tea.Cmd {
	if workspaceCount == 0 {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, d.keys.Down):
			d.selected++
			if d.selected >= workspaceCount {
				d.selected = 0
			}
		case key.Matches(msg, d.keys.Up):
			d.selected--
			if d.selected < 0 {
				d.selected = workspaceCount - 1
			}
		case key.Matches(msg, d.keys.Right):
			d.selected++
			if d.selected >= workspaceCount {
				d.selected = 0
			}
		case key.Matches(msg, d.keys.Left):
			d.selected--
			if d.selected < 0 {
				d.selected = workspaceCount - 1
			}
		}
	}

	return nil
}

// View renders the dashboard grid of workspace panels.
func (d *DashboardModel) View(workspaces []*agent.Workspace, width, height int, minPanelWidth int) string {
	if len(workspaces) == 0 {
		msg := "No workspaces yet. Press n to create one."
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
	}

	// Clamp the selected index.
	if d.selected >= len(workspaces) {
		d.selected = len(workspaces) - 1
	}
	if d.selected < 0 {
		d.selected = 0
	}

	if minPanelWidth <= 0 {
		minPanelWidth = 40
	}

	// Calculate grid layout.
	columns := width / minPanelWidth
	if columns > len(workspaces) {
		columns = len(workspaces)
	}
	if columns < 1 {
		columns = 1
	}

	gap := 2
	panelWidth := (width - (columns-1)*gap) / columns
	if panelWidth < minPanelWidth && columns > 1 {
		columns = 1
		panelWidth = width
	}

	rows := (len(workspaces) + columns - 1) / columns
	panelHeight := (height - 2) / rows
	if panelHeight < 5 {
		panelHeight = 5
	}

	// Build grid rows.
	var rowStrings []string
	for row := 0; row < rows; row++ {
		var panels []string
		for col := 0; col < columns; col++ {
			idx := row*columns + col
			if idx >= len(workspaces) {
				// Render empty filler panel so the grid stays aligned.
				panels = append(panels, strings.Repeat(" ", panelWidth))
				continue
			}
			active := idx == d.selected
			panels = append(panels, renderPanel(workspaces[idx], panelWidth, panelHeight, active))
		}
		rowStrings = append(rowStrings, lipgloss.JoinHorizontal(lipgloss.Top, panels...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rowStrings...)
}

// renderPanel renders a single workspace panel.
func renderPanel(ws *agent.Workspace, width, height int, active bool) string {
	style := panelStyle
	if active {
		style = activePanelStyle
	}

	// Account for border and padding in available content dimensions.
	// Rounded border is 1 char on each side; padding is 0 top/bottom, 1 left/right.
	contentWidth := width - 4  // 2 border + 2 padding
	contentHeight := height - 2 // 2 border
	if contentWidth < 10 {
		contentWidth = 10
	}
	if contentHeight < 3 {
		contentHeight = 3
	}

	var lines []string

	// Line 1: task name (truncated).
	task := ws.Task
	if len(task) > contentWidth {
		task = task[:contentWidth-1] + "…"
	}
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render(task))

	// Line 2: backend + model + status icon.
	backendInfo := ""
	if ws.Backend != nil {
		backendInfo = ws.Backend.Name
		if ws.Backend.Model != "" {
			backendInfo += " / " + ws.Backend.Model
		}
	}
	statusIcon := statusIconFor(ws.Status)
	line2 := backendInfo
	if len(line2)+3 > contentWidth {
		line2 = line2[:contentWidth-4] + "…"
	}
	line2 = line2 + " " + statusIcon
	lines = append(lines, line2)

	// Line 3: turn count + elapsed time.
	elapsed := time.Since(ws.CreatedAt).Truncate(time.Second)
	minutes := int(elapsed.Minutes())
	seconds := int(elapsed.Seconds()) % 60
	line3 := fmt.Sprintf("Turn %d", ws.TurnCount)
	if minutes > 0 {
		line3 += fmt.Sprintf(" • %dm %ds", minutes, seconds)
	} else {
		line3 += fmt.Sprintf(" • %ds", seconds)
	}
	lines = append(lines, turnSeparatorStyle.Render(line3))

	// Lines 4+: last N lines of output.
	outputLines := contentHeight - len(lines)
	if outputLines > 0 && ws.Output != nil {
		last := ws.Output.Last(outputLines)
		for _, l := range last {
			if len(l) > contentWidth {
				l = l[:contentWidth-1] + "…"
			}
			lines = append(lines, l)
		}
	}

	// Pad to fill contentHeight.
	for len(lines) < contentHeight {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	return style.
		Width(width - 2). // subtract border width
		Height(contentHeight).
		Render(content)
}

// statusIconFor returns a styled status icon character for the given status.
func statusIconFor(s agent.WorkspaceStatus) string {
	switch s {
	case agent.StatusInitializing:
		return waitingStyle.Render("…")
	case agent.StatusRunning:
		return runningStyle.Render("●")
	case agent.StatusWaiting:
		return waitingStyle.Render("○")
	case agent.StatusCompleted:
		return completedStyle.Render("✓")
	case agent.StatusErrored:
		return erroredStyle.Render("✗")
	case agent.StatusCancelled:
		return lipgloss.NewStyle().Faint(true).Render("◌")
	case agent.StatusMerging:
		return runningStyle.Render("⟳")
	default:
		return "?"
	}
}
