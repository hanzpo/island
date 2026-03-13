package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
)

// WorkspaceViewModel renders the right panel showing the active workspace's
// output and input bar.
type WorkspaceViewModel struct {
	viewport     viewport.Model
	input        textinput.Model
	autoScroll   bool
	keys         WorkspaceKeyMap
	ready        bool
	lastWSID     string // tracks if workspace changed
	lastSessID   string // tracks if session changed
	spinnerFrame int    // current spinner animation frame
}

func newWorkspaceViewModel() WorkspaceViewModel {
	ti := textinput.New()
	ti.Placeholder = "Send follow-up..."
	ti.CharLimit = 1000

	return WorkspaceViewModel{
		autoScroll: true,
		keys:       defaultWorkspaceKeys(),
		input:      ti,
	}
}

// Focus is called when the selected workspace changes. It populates the
// viewport from the active session's output ring buffer.
func (w *WorkspaceViewModel) Focus(ws *agent.Workspace, width, height int) {
	if ws == nil {
		return
	}

	sess := ws.ActiveSession()
	w.lastWSID = ws.ID
	if sess != nil {
		w.lastSessID = sess.ID
	} else {
		w.lastSessID = ""
	}
	w.autoScroll = true

	headerHeight := 2 // session header + separator
	inputHeight := 3  // border + input + padding
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

	// Populate viewport from the active session's output.
	w.viewport.SetContent(sessionContent(sess, w.spinnerFrame))
	if w.autoScroll {
		w.viewport.GotoBottom()
	}

	w.input.Width = width - 4

	w.input.Focus()
}

// Update handles messages routed to the workspace view.
func (w *WorkspaceViewModel) Update(msg tea.Msg, ws *agent.Workspace) tea.Cmd {
	var cmds []tea.Cmd

	// Check if workspace or session changed.
	if ws != nil {
		sess := ws.ActiveSession()
		newWSID := ws.ID
		newSessID := ""
		if sess != nil {
			newSessID = sess.ID
		}
		if newWSID != w.lastWSID || newSessID != w.lastSessID {
			w.lastWSID = newWSID
			w.lastSessID = newSessID
			w.viewport.SetContent(sessionContent(sess, w.spinnerFrame))
			w.autoScroll = true
			w.viewport.GotoBottom()
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if w.input.Focused() {
			// Pass to text input.
			var cmd tea.Cmd
			w.input, cmd = w.input.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return tea.Batch(cmds...)
		}

		// Input not focused -- handle navigation keys.
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

// UpdateOutput refreshes the viewport content from the active session's
// output ring buffer. Called when a new OutputMsg arrives for the current
// session. The ring buffer is the single source of truth -- the TUI handler
// does NOT write to the ring buffer, just reads from it.
func (w *WorkspaceViewModel) UpdateOutput(ws *agent.Workspace) {
	if ws == nil {
		return
	}
	sess := ws.ActiveSession()
	w.viewport.SetContent(sessionContent(sess, w.spinnerFrame))
	if w.autoScroll {
		w.viewport.GotoBottom()
	}
}

// View renders the workspace panel content for the given workspace.
func (w *WorkspaceViewModel) View(ws *agent.Workspace, width, height int) string {
	if ws == nil {
		return ""
	}

	var b strings.Builder

	// Session header.
	b.WriteString(w.renderSessionHeader(ws, width))
	b.WriteByte('\n')

	// Separator.
	sep := lipgloss.NewStyle().Faint(true).Render(strings.Repeat("\u2500", width))
	b.WriteString(sep)
	b.WriteByte('\n')

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

	// Always show the input bar.
	b.WriteString(inputStyle.Width(width - 2).Render(w.input.View()))

	return b.String()
}

// renderSessionHeader builds the header line for the workspace's right panel.
// If the workspace has multiple sessions, it renders tabs. Otherwise, it shows
// the agent name and status inline.
func (w *WorkspaceViewModel) renderSessionHeader(ws *agent.Workspace, width int) string {
	sess := ws.ActiveSession()
	taskName := ws.Name
	if len(taskName) > width/2 {
		taskName = taskName[:width/2-1] + "\u2026"
	}

	if len(ws.Sessions) <= 1 {
		// Single session: "agent_name <status_icon>  task_name"
		agentName := ""
		icon := ""
		if sess != nil {
			if sess.Agent != nil {
				agentName = sess.Agent.Name
			}
			icon = statusIconFor(sess.Status, w.spinnerFrame)
		}
		left := headerStyle.Render(taskName)
		right := footerStyle.Render(agentName) + " " + icon

		gap := width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}
		return left + strings.Repeat(" ", gap) + right
	}

	// Multiple sessions: render tabs.
	var tabs strings.Builder
	for i, s := range ws.Sessions {
		agentName := ""
		if s.Agent != nil {
			agentName = s.Agent.Name
		}
		icon := statusIconFor(s.Status, w.spinnerFrame)
		label := fmt.Sprintf("[%d:%s %s]", i+1, agentName, icon)

		if i == ws.ActiveIdx {
			tabs.WriteString(sessionActiveTabStyle.Render(label))
		} else {
			tabs.WriteString(sessionTabStyle.Render(label))
		}
	}

	left := tabs.String()
	right := headerStyle.Render(taskName)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// sessionContent joins all output lines from a session for display in the
// viewport. Appends a thinking indicator when the agent is active, or a
// completion/error indicator when done.
func sessionContent(sess *agent.Session, spinnerFrame int) string {
	if sess == nil || sess.Output == nil {
		return ""
	}
	lines := sess.Output.Lines()
	content := strings.Join(lines, "\n")

	// Append status indicator based on session state.
	switch sess.Status {
	case agent.StatusRunning:
		f := spinnerFrames[spinnerFrame%len(spinnerFrames)]
		content += "\n" + thinkingStyle.Render(f+" Thinking...")
	case agent.StatusCompleted:
		content += "\n" + completedStyle.Render("\u2713 Task completed")
	case agent.StatusErrored:
		errMsg := "Agent exited with error"
		if sess.Error != nil {
			errMsg += ": " + sess.Error.Error()
		}
		content += "\n" + erroredStyle.Render("\u2717 "+errMsg)
	}

	return content
}
