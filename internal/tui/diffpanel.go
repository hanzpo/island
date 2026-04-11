package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
)

// DiffPanelModel renders a diff in a split panel.
type DiffPanelModel struct {
	workspaceID string
	viewport    viewport.Model
	diff        string
	ready       bool
	open        bool
}

func newDiffPanelModel() DiffPanelModel {
	return DiffPanelModel{}
}

// IsOpen returns whether the diff panel is visible.
func (d *DiffPanelModel) IsOpen() bool {
	return d.open
}

// Toggle opens or closes the diff panel.
func (d *DiffPanelModel) Toggle() {
	d.open = !d.open
}

// Close closes the diff panel.
func (d *DiffPanelModel) Close() {
	d.open = false
}

// SetDiff populates the diff panel with content.
func (d *DiffPanelModel) SetDiff(workspaceID, diff string, width, height int) {
	d.workspaceID = workspaceID
	d.diff = diff
	d.open = true

	vpHeight := height
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !d.ready {
		d.viewport = viewport.New(width, vpHeight)
		d.viewport.MouseWheelEnabled = false
		d.ready = true
	} else {
		d.viewport.Width = width
		d.viewport.Height = vpHeight
	}

	d.viewport.SetContent(colorDiff(diff))
	d.viewport.GotoTop()
}

// Resize adjusts the diff viewport dimensions.
func (d *DiffPanelModel) Resize(width, height int) {
	if !d.ready {
		return
	}
	d.viewport.Width = width
	d.viewport.Height = height
}

// ScrollUp scrolls the diff viewport up.
func (d *DiffPanelModel) ScrollUp(lines int) {
	d.viewport.LineUp(lines)
}

// ScrollDown scrolls the diff viewport down.
func (d *DiffPanelModel) ScrollDown(lines int) {
	d.viewport.LineDown(lines)
}

// PageUp scrolls up by half a page.
func (d *DiffPanelModel) PageUp() {
	d.viewport.HalfViewUp()
}

// PageDown scrolls down by half a page.
func (d *DiffPanelModel) PageDown() {
	d.viewport.HalfViewDown()
}

// GotoTop scrolls to the top.
func (d *DiffPanelModel) GotoTop() {
	d.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom.
func (d *DiffPanelModel) GotoBottom() {
	d.viewport.GotoBottom()
}

// View renders the diff viewport content (without border).
func (d *DiffPanelModel) View() string {
	if !d.ready || !d.open {
		return ""
	}
	return d.viewport.View()
}

// colorDiff applies syntax coloring to a unified diff string.
func colorDiff(diff string) string {
	if diff == "" {
		return styleSubtle.Render("(no changes)")
	}

	lines := strings.Split(diff, "\n")
	var b strings.Builder

	for i, line := range lines {
		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = styleDiffHeader.Render(line)
		case strings.HasPrefix(line, "+"):
			styled = styleDiffAdded.Render(line)
		case strings.HasPrefix(line, "-"):
			styled = styleDiffRemoved.Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = styleDiffHunk.Render(line)
		case strings.HasPrefix(line, "diff "):
			styled = styleDiffHeader.Render(line)
		default:
			styled = styleDiffContext.Render(line)
		}
		b.WriteString(styled)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}
