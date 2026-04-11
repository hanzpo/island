package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/hanz/island/internal/agent"
)

// ansiRegex matches ANSI escape sequences for stripping from stored content.
var ansiRegex = regexp.MustCompile(`\x1b\[[\x20-\x3f]*[\x40-\x7e]`)

// OutputModel renders the agent output viewport with session tabs and input bar.
type OutputModel struct {
	viewport     viewport.Model
	input        textinput.Model
	autoScroll   bool
	ready        bool
	lastWSID     string
	lastSessID   string
	spinnerFrame int
}

func newOutputModel() OutputModel {
	ti := textinput.New()
	ti.Placeholder = "What would you like to work on?"
	ti.CharLimit = 2000

	return OutputModel{
		autoScroll: true,
		input:      ti,
	}
}

// Focus is called when the selected workspace changes.
func (o *OutputModel) Focus(ws *agent.Workspace, width, height int) {
	if ws == nil {
		return
	}

	sess := ws.ActiveSession()
	o.lastWSID = ws.ID
	if sess != nil {
		o.lastSessID = sess.ID
	} else {
		o.lastSessID = ""
	}
	o.autoScroll = true

	// Reserve a line for tabs only when there are multiple sessions.
	vpHeight := height
	if len(ws.Sessions) > 1 {
		vpHeight--
	}
	if vpHeight < 1 {
		vpHeight = 1
	}

	if !o.ready {
		o.viewport = viewport.New(width, vpHeight)
		o.viewport.MouseWheelEnabled = false
		o.ready = true
	} else {
		o.viewport.Width = width
		o.viewport.Height = vpHeight
	}

	o.viewport.SetContent(sessionContent(sess, o.spinnerFrame, o.viewport.Width))
	if o.autoScroll {
		o.viewport.GotoBottom()
	}

	o.input.Width = width - 4

	if ws.Archived {
		o.input.Placeholder = ""
		o.input.Blur()
	} else {
		o.input.Placeholder = "Send a message..."
	}
}

// Update handles messages routed to the output panel.
func (o *OutputModel) Update(msg tea.Msg, ws *agent.Workspace) tea.Cmd {
	if ws != nil {
		sess := ws.ActiveSession()
		newWSID := ws.ID
		newSessID := ""
		if sess != nil {
			newSessID = sess.ID
		}
		if newWSID != o.lastWSID || newSessID != o.lastSessID {
			o.lastWSID = newWSID
			o.lastSessID = newSessID
			o.viewport.SetContent(sessionContent(sess, o.spinnerFrame, o.viewport.Width))
			o.autoScroll = true
			o.viewport.GotoBottom()
		}
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if o.input.Focused() {
			var cmd tea.Cmd
			o.input, cmd = o.input.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return tea.Batch(cmds...)
		}
	}

	var vpCmd tea.Cmd
	o.viewport, vpCmd = o.viewport.Update(msg)
	if vpCmd != nil {
		cmds = append(cmds, vpCmd)
	}

	if o.viewport.AtBottom() {
		o.autoScroll = true
	}

	return tea.Batch(cmds...)
}

// UpdateOutput refreshes viewport from the session's ring buffer.
func (o *OutputModel) UpdateOutput(ws *agent.Workspace) {
	if ws == nil {
		return
	}
	sess := ws.ActiveSession()
	o.viewport.SetContent(sessionContent(sess, o.spinnerFrame, o.viewport.Width))
	if o.autoScroll {
		o.viewport.GotoBottom()
	}
}

// ScrollUp scrolls the output viewport up.
func (o *OutputModel) ScrollUp(lines int) {
	o.autoScroll = false
	o.viewport.LineUp(lines)
}

// ScrollDown scrolls the output viewport down.
func (o *OutputModel) ScrollDown(lines int) {
	o.viewport.LineDown(lines)
	if o.viewport.AtBottom() {
		o.autoScroll = true
	}
}

// PageUp scrolls the output viewport up by half a page.
func (o *OutputModel) PageUp() {
	o.autoScroll = false
	o.viewport.HalfViewUp()
}

// PageDown scrolls the output viewport down by half a page.
func (o *OutputModel) PageDown() {
	o.viewport.HalfViewDown()
	if o.viewport.AtBottom() {
		o.autoScroll = true
	}
}

// GotoTop scrolls to the top.
func (o *OutputModel) GotoTop() {
	o.autoScroll = false
	o.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom.
func (o *OutputModel) GotoBottom() {
	o.viewport.GotoBottom()
	o.autoScroll = true
}

// ViewTabs renders the session tabs for a workspace.
func (o *OutputModel) ViewTabs(ws *agent.Workspace, width int) string {
	if ws == nil || len(ws.Sessions) == 0 {
		return ""
	}

	// Single session — no tabs needed.
	if len(ws.Sessions) == 1 {
		return ""
	}

	var parts []string
	for i, s := range ws.Sessions {
		agentName := ""
		if s.Agent != nil {
			agentName = s.Agent.Name
		}
		icon := statusIconChar(s.Status, o.spinnerFrame)
		label := fmt.Sprintf("%d:%s %s", i+1, agentName, icon)

		var styled string
		if i == ws.ActiveIdx {
			styled = styleTabActive.Render(label)
		} else {
			styled = styleTab.Render(label)
		}
		parts = append(parts, zones.Mark(sessionTabZoneID(i), styled))
	}

	sep := styleSubtle.Render(" │ ")
	return strings.Join(parts, sep)
}

// ViewOutput renders the viewport content.
func (o *OutputModel) ViewOutput() string {
	return o.viewport.View()
}

// ViewInput renders the input bar.
func (o *OutputModel) ViewInput(width int, active bool) string {
	// width is total available. Rounded border adds 2 (left+right border),
	// padding adds 2 (1 each side). Content width = total - 4.
	cw := width - 4
	if cw < 10 {
		cw = 10
	}
	o.input.Width = cw
	style := styleInput
	if active {
		style = style.BorderForeground(colorBorderActive)
	}
	return style.Width(cw).Render(o.input.View())
}

// --- Content Rendering ---

// lineKind classifies output lines for rendering.
type lineKind int

const (
	lineText       lineKind = iota
	lineTool
	lineUser
	lineStatus
	lineError
	lineCompletion
)

func classifyLine(line string) lineKind {
	trimmed := strings.TrimLeft(line, " ")
	switch {
	case strings.HasPrefix(trimmed, "\u25cf "):
		return lineTool
	case strings.HasPrefix(trimmed, "\u2713 "):
		if strings.HasPrefix(line, " ") {
			return lineTool
		}
		return lineCompletion
	case strings.HasPrefix(line, "\u276f "):
		return lineUser
	case strings.HasPrefix(line, "\u29d6 "):
		return lineStatus
	case strings.HasPrefix(line, "\u2717 "):
		return lineError
	default:
		return lineText
	}
}

func sessionContent(sess *agent.Session, spinnerFrame int, width int) string {
	if sess == nil || sess.Output == nil {
		return ""
	}
	lines := sess.Output.Lines()

	if len(lines) == 0 {
		return ""
	}

	type segment struct {
		kind  lineKind
		lines []string
	}
	var segments []segment
	var currentText []string

	flushText := func() {
		if len(currentText) > 0 {
			segments = append(segments, segment{kind: lineText, lines: currentText})
			currentText = nil
		}
	}

	for _, line := range lines {
		clean := ansiRegex.ReplaceAllString(line, "")
		kind := classifyLine(clean)
		if kind != lineText {
			flushText()
			segments = append(segments, segment{kind: kind, lines: []string{clean}})
		} else {
			currentText = append(currentText, clean)
		}
	}
	flushText()

	var b strings.Builder
	for i, seg := range segments {
		if i > 0 {
			// Add a blank line before user messages for turn separation.
			if seg.kind == lineUser {
				b.WriteString("\n\n")
			} else {
				b.WriteByte('\n')
			}
		}
		switch seg.kind {
		case lineTool:
			b.WriteString(styleToolLine.Render(seg.lines[0]))
		case lineUser:
			b.WriteString(styleUserMsg.Render(seg.lines[0]))
		case lineStatus:
			b.WriteString(styleThinking.Render(strings.TrimPrefix(seg.lines[0], "\u29d6 ")))
		case lineError:
			b.WriteString(styleErrored.Render(seg.lines[0]))
		case lineCompletion:
			b.WriteString(styleCompleted.Render(seg.lines[0]))
		default:
			text := strings.Join(seg.lines, "\n")
			b.WriteString(renderMarkdown(text, width))
		}
	}

	content := b.String()

	if sess.Status == agent.StatusRunning {
		f := spinnerFrames[spinnerFrame%len(spinnerFrames)]
		content += "\n" + styleThinking.Render(f+" Thinking...")
	} else if sess.Status == agent.StatusErrored {
		errMsg := "agent exited with error"
		if sess.Error != nil {
			errMsg += ": " + sess.Error.Error()
		}
		content += "\n\n" + styleErrored.Render(errMsg)
	}

	return content
}

// hasMarkdown returns true if the content contains markdown syntax worth rendering.
func hasMarkdown(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") ||
			strings.HasPrefix(trimmed, "# ") ||
			strings.HasPrefix(trimmed, "## ") ||
			strings.HasPrefix(trimmed, "- ") ||
			strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "> ") ||
			strings.HasPrefix(trimmed, "| ") ||
			strings.Contains(trimmed, "](") {
			return true
		}
	}
	return false
}

func renderMarkdown(content string, width int) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	if width <= 0 {
		width = 80
	}

	// Plain text — skip glamour to avoid its padding/margins.
	if !hasMarkdown(content) {
		return content
	}

	content = closeUnclosedFences(content)

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}

	rendered, err := r.Render(content)
	if err != nil {
		return content
	}

	// Glamour adds leading/trailing newlines and 2-space indent — strip them.
	rendered = strings.TrimRight(rendered, "\n ")
	rendered = strings.TrimLeft(rendered, "\n")

	return rendered
}

func closeUnclosedFences(content string) string {
	fenceCount := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenceCount++
		}
	}
	if fenceCount%2 != 0 {
		content += "\n```"
	}
	return content
}
