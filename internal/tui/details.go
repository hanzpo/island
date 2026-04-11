package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// DetailsModel renders the workspace details panel.
type DetailsModel struct{}

func newDetailsModel() DetailsModel {
	return DetailsModel{}
}

// View renders workspace details. Width and height are the content area inside the border.
func (d *DetailsModel) View(ws *agent.Workspace, width, height, spinnerFrame int) string {
	if ws == nil || width <= 0 || height <= 0 {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			styleSubtle.Render("No workspace selected"))
	}

	var lines []string

	// Agent name
	sess := ws.ActiveSession()
	if sess != nil && sess.Agent != nil {
		lines = append(lines, renderDetailLine("Agent", sess.Agent.Name, width))
	}

	// Status
	statusIcon := statusIconStyled(ws.Status(), spinnerFrame)
	statusText := ws.Status().String()
	lines = append(lines, renderDetailLine("Status", statusIcon+" "+statusText, width))

	// Branch
	if ws.Branch != "" {
		branch := ws.Branch
		maxBranchLen := width - 10
		if maxBranchLen > 0 && len(branch) > maxBranchLen {
			branch = branch[:maxBranchLen-1] + "\u2026"
		}
		lines = append(lines, renderDetailLine("Branch", branch, width))
	}

	// Task (truncated)
	if sess != nil && sess.Task != "" {
		task := sess.Task
		maxTaskLen := width - 8
		if maxTaskLen > 0 && len(task) > maxTaskLen {
			task = task[:maxTaskLen-1] + "\u2026"
		}
		lines = append(lines, renderDetailLine("Task", task, width))
	}

	// PR info
	if ws.PRNumber > 0 {
		prText := fmt.Sprintf("#%d", ws.PRNumber)
		lines = append(lines, renderDetailLine("PR", styleBadge.Render(prText), width))
	}

	// Elapsed time
	if sess != nil && sess.Status == agent.StatusRunning {
		elapsed := time.Since(sess.CreatedAt).Truncate(time.Second)
		lines = append(lines, renderDetailLine("Time", elapsed.String(), width))
	}

	// Sessions count
	if len(ws.Sessions) > 1 {
		lines = append(lines, renderDetailLine("Sessions",
			fmt.Sprintf("%d/%d", ws.ActiveIdx+1, len(ws.Sessions)), width))
	}

	content := strings.Join(lines, "\n")

	// Pad to fill height.
	lineCount := len(lines)
	for lineCount < height {
		content += "\n"
		lineCount++
	}

	return content
}

func renderDetailLine(label, value string, width int) string {
	l := styleLabel.Render(label + ":")
	v := " " + value
	// Truncate value if it would overflow.
	maxValueWidth := width - lipgloss.Width(l) - 1
	if maxValueWidth > 0 && lipgloss.Width(v) > maxValueWidth {
		v = v[:maxValueWidth]
	}
	return l + v
}
