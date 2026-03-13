package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DiffViewModel is the full-screen diff review screen with syntax-colored
// output.
type DiffViewModel struct {
	workspaceID   string
	workspaceName string
	viewport      viewport.Model
	diff          string
	keys          DiffKeyMap
	ready         bool
}

func newDiffViewModel() DiffViewModel {
	return DiffViewModel{
		keys: defaultDiffKeys(),
	}
}

// SetDiff populates the diff view with the given diff content.
func (d *DiffViewModel) SetDiff(workspaceID, workspaceName, diff string, width, height int) {
	d.workspaceID = workspaceID
	d.workspaceName = workspaceName
	d.diff = diff

	headerHeight := 2 // header + separator
	footerHeight := 1
	vpHeight := height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !d.ready {
		d.viewport = viewport.New(width, vpHeight)
		d.ready = true
	} else {
		d.viewport.Width = width
		d.viewport.Height = vpHeight
	}

	colored := colorDiff(diff)
	d.viewport.SetContent(colored)
	d.viewport.GotoTop()
}

// Update handles messages in the diff view.
func (d *DiffViewModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, d.keys.PageUp):
			d.viewport.HalfViewUp()
			return nil
		case key.Matches(msg, d.keys.PageDown):
			d.viewport.HalfViewDown()
			return nil
		case key.Matches(msg, d.keys.Top):
			d.viewport.GotoTop()
			return nil
		case key.Matches(msg, d.keys.Bottom):
			d.viewport.GotoBottom()
			return nil
		}
	}

	// Propagate to viewport for scrolling.
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return cmd
}

// View renders the diff review screen.
func (d *DiffViewModel) View(width, height int) string {
	var b strings.Builder

	// Header.
	title := headerStyle.Render("Diff Review")
	if d.workspaceName != "" {
		title += footerStyle.Render("  " + d.workspaceName)
	} else if d.workspaceID != "" {
		title += footerStyle.Render("  " + d.workspaceID)
	}
	b.WriteString(title)
	b.WriteByte('\n')

	// Separator.
	sep := lipgloss.NewStyle().Faint(true).Render(strings.Repeat("\u2500", width))
	b.WriteString(sep)
	b.WriteByte('\n')

	// Viewport.
	headerHeight := 2
	footerHeight := 1
	vpHeight := height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	if d.viewport.Width != width || d.viewport.Height != vpHeight {
		d.viewport.Width = width
		d.viewport.Height = vpHeight
	}
	b.WriteString(d.viewport.View())
	b.WriteByte('\n')

	// Footer.
	footer := footerStyle.Render("  m: merge  x: discard  esc: back")
	b.WriteString(footer)

	return b.String()
}

// colorDiff applies syntax coloring to a unified diff string.
func colorDiff(diff string) string {
	if diff == "" {
		return "(no changes)"
	}

	lines := strings.Split(diff, "\n")
	var b strings.Builder

	for i, line := range lines {
		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = diffHeaderStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			styled = diffAddedStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			styled = diffRemovedStyle.Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = diffHunkStyle.Render(line)
		case strings.HasPrefix(line, "diff "):
			styled = diffHeaderStyle.Render(line)
		default:
			styled = diffContextStyle.Render(line)
		}
		b.WriteString(styled)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}
