package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// WorkspaceModel is the full-screen workspace focus view with streaming output
// and a follow-up input bar.
type WorkspaceModel struct {
	workspaceID string
	viewport    viewport.Model
	input       textinput.Model
	autoScroll  bool
	keys        WorkspaceKeyMap
	ready       bool
}

func newWorkspaceModel() WorkspaceModel {
	ti := textinput.New()
	ti.Placeholder = "Send follow-up..."
	ti.CharLimit = 1000

	return WorkspaceModel{
		autoScroll: true,
		keys:       defaultWorkspaceKeys(),
		input:      ti,
	}
}

// Focus switches to viewing the given workspace.
func (w *WorkspaceModel) Focus(ws *agent.Workspace, width, height int) {
	w.workspaceID = ws.ID
	w.autoScroll = true

	headerHeight := 2  // header + blank line
	inputHeight := 3   // border + input line + padding
	vpHeight := height - headerHeight - inputHeight
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !w.ready {
		w.viewport = viewport.New(width, vpHeight)
		w.viewport.YPosition = headerHeight
		w.ready = true
	} else {
		w.viewport.Width = width
		w.viewport.Height = vpHeight
	}

	// Populate viewport with current output.
	w.viewport.SetContent(workspaceContent(ws))
	if w.autoScroll {
		w.viewport.GotoBottom()
	}

	w.input.Width = width - 4
	w.input.Focus()
}

// Update handles messages in the workspace view.
func (w *WorkspaceModel) Update(msg tea.Msg, ws *agent.Workspace) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If the input is focused and the user is typing, only intercept
		// specific bindings.
		if w.input.Focused() {
			switch {
			case key.Matches(msg, w.keys.Back):
				w.input.Blur()
				return nil
			case key.Matches(msg, w.keys.Cancel):
				// Let 'c' go to input unless ctrl-based.
				// We only cancel when the input is empty.
				if w.input.Value() == "" && ws != nil && ws.Cancel != nil {
					return nil // App.Update handles the cancel action
				}
			}

			// Pass to text input.
			var cmd tea.Cmd
			w.input, cmd = w.input.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return tea.Batch(cmds...)
		}

		// Input not focused — handle navigation keys.
		switch {
		case key.Matches(msg, w.keys.PageUp):
			w.autoScroll = false
			w.viewport.HalfViewUp()
		case key.Matches(msg, w.keys.PageDown):
			w.viewport.HalfViewDown()
			if w.viewport.AtBottom() {
				w.autoScroll = true
			}
		case key.Matches(msg, w.keys.Top):
			w.autoScroll = false
			w.viewport.GotoTop()
		case key.Matches(msg, w.keys.Bottom):
			w.viewport.GotoBottom()
			w.autoScroll = true
		}
	}

	// Propagate to viewport for mouse/scroll.
	var vpCmd tea.Cmd
	w.viewport, vpCmd = w.viewport.Update(msg)
	if vpCmd != nil {
		cmds = append(cmds, vpCmd)
	}

	// Track autoscroll after viewport update.
	if w.viewport.AtBottom() {
		w.autoScroll = true
	}

	return tea.Batch(cmds...)
}

// UpdateOutput refreshes the viewport content from the workspace output buffer.
func (w *WorkspaceModel) UpdateOutput(ws *agent.Workspace) {
	if ws == nil {
		return
	}
	w.viewport.SetContent(workspaceContent(ws))
	if w.autoScroll {
		w.viewport.GotoBottom()
	}
}

// View renders the workspace focus screen.
func (w *WorkspaceModel) View(ws *agent.Workspace, width, height int) string {
	if ws == nil {
		return ""
	}

	var b strings.Builder

	// Header.
	task := ws.Task
	if len(task) > width-20 {
		task = task[:width-21] + "…"
	}
	backendInfo := ""
	if ws.Backend != nil {
		backendInfo = ws.Backend.Name
		if ws.Backend.Model != "" {
			backendInfo += " / " + ws.Backend.Model
		}
	}
	elapsed := time.Since(ws.CreatedAt).Truncate(time.Second)
	statusStr := statusIconFor(ws.Status) + " " + ws.Status.String()

	header := headerStyle.Render(task) + "  " +
		footerStyle.Render(backendInfo) + "  " +
		statusStr + "  " +
		footerStyle.Render(fmt.Sprintf("Turn %d  %s", ws.TurnCount, elapsed))
	b.WriteString(header)
	b.WriteByte('\n')

	// Separator.
	sep := lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", width))
	b.WriteString(sep)
	b.WriteByte('\n')

	// Viewport.
	// Resize viewport if dimensions changed.
	headerHeight := 2
	inputHeight := 3
	vpHeight := height - headerHeight - inputHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	if w.viewport.Width != width || w.viewport.Height != vpHeight {
		w.viewport.Width = width
		w.viewport.Height = vpHeight
	}
	b.WriteString(w.viewport.View())
	b.WriteByte('\n')

	// Input bar (only show if workspace can accept follow-up).
	canInput := ws.Status == agent.StatusWaiting || ws.Status == agent.StatusCompleted
	if canInput {
		b.WriteString(inputStyle.Width(width - 2).Render(w.input.View()))
	} else {
		hint := footerStyle.Render(fmt.Sprintf("  esc: back  d: diff  c: cancel"))
		b.WriteString(hint)
	}

	return b.String()
}

// workspaceContent joins all output lines for display in the viewport.
func workspaceContent(ws *agent.Workspace) string {
	if ws.Output == nil {
		return ""
	}
	lines := ws.Output.Lines()
	return strings.Join(lines, "\n")
}
