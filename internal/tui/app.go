package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
	"github.com/hanz/island/internal/config"
	"github.com/hanz/island/internal/git"
	"github.com/hanz/island/internal/hooks"
	"github.com/hanz/island/internal/state"
)

// --- Message types ---

type InitOutputMsg struct {
	WorkspaceID string
	Line        string
}

type DiffReadyMsg struct {
	WorkspaceID string
	Diff        string
	Err         error
}

type MergeCompleteMsg struct {
	WorkspaceID string
	Err         error
}

type DiscardCompleteMsg struct {
	WorkspaceID string
	Err         error
}

type TickMsg struct{}

type AnimTickMsg struct{}

type WorkspaceRenameMsg struct {
	WorkspaceID string
	NewName     string
	NewBranch   string
	Err         error
}

type PRCreateCompleteMsg struct {
	WorkspaceID string
	PRNumber    int
	PRURL       string
	Err         error
}


// App is the root Bubble Tea model.
type App struct {
	// Dependencies
	cfg        *config.Config
	gitMgr     *git.Manager
	hookRunner *hooks.Runner
	pool       *agent.Pool
	program    *tea.Program
	repoRoot   string
	repoName   string

	// Persistence
	statePath  string
	historyDir string

	// State
	workspaces []*agent.Workspace
	selected   int

	// Layout
	layout Layout
	focus  PanelID
	keys   Keys

	// Sub-models
	sidebar  SidebarModel
	details  DetailsModel
	output   OutputModel
	diff     DiffPanelModel
	dialog   DialogModel
	help     HelpModel

	// Window
	width  int
	height int

	// Animation
	spinnerFrame int
	animating    bool

	// Confirmation
	confirmQuit    bool
	confirmDiscard bool

	// Mouse
	lastScrollAt time.Time
}

// NewApp creates and initializes the root TUI model.
func NewApp(cfg *config.Config, repoRoot string) *App {
	gitMgr := git.NewManager(repoRoot, cfg.General.WorktreeDir)
	hookRunner := hooks.NewRunner(repoRoot)
	pool := agent.NewPool(cfg.General.MaxConcurrent)

	app := &App{
		cfg:        cfg,
		gitMgr:     gitMgr,
		hookRunner: hookRunner,
		pool:       pool,
		repoRoot:   repoRoot,
		repoName:   filepath.Base(repoRoot),
		statePath:  filepath.Join(repoRoot, ".island", "state.json"),
		historyDir: filepath.Join(repoRoot, ".island", "history"),
		focus:      PanelWorkspaces,
		keys:       DefaultKeys(),
		sidebar:    newSidebarModel(),
		details:    newDetailsModel(),
		output:     newOutputModel(),
		diff:       newDiffPanelModel(),
		dialog:     newDialogModel(cfg),
		help:       newHelpModel(),
	}

	if s, err := state.Load(app.statePath); err == nil && s != nil {
		app.restoreState(s)
	}

	return app
}

// SetProgram stores a reference to the tea.Program.
func (m *App) SetProgram(p *tea.Program) {
	m.program = p
}

// Init implements tea.Model.
func (m *App) Init() tea.Cmd {
	return tickCmd()
}

// Update implements tea.Model.
func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout = ComputeLayout(m.width, m.height, m.diff.IsOpen())
		m.focusSelected()
		return m, nil

	case TickMsg:
		return m, tickCmd()

	case AnimTickMsg:
		return m.handleAnimTick()

	case agent.OutputMsg:
		return m.handleOutputMsg(msg)

	case agent.DoneMsg:
		return m.handleDoneMsg(msg)

	case DiffReadyMsg:
		return m.handleDiffReady(msg)

	case MergeCompleteMsg:
		return m.handleMergeComplete(msg)

	case DiscardCompleteMsg:
		return m.handleDiscardComplete(msg)

	case WorkspaceRenameMsg:
		return m.handleWorkspaceRename(msg)

	case PRCreateCompleteMsg:
		return m.handlePRCreateComplete(msg)

	case tea.MouseMsg:
		return m.handleMouseMsg(msg)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

// View implements tea.Model.
func (m *App) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Help overlay.
	if m.help.IsOpen() {
		return m.help.View(m.width, m.height)
	}

	// Dialog overlay.
	if m.dialog.IsOpen() {
		return m.dialog.View(m.width, m.height)
	}

	// Quit confirmation.
	if m.confirmQuit {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleDialog.Render("Agents are still running. Quit anyway? (y/n)"))
	}

	// Discard confirmation.
	if m.confirmDiscard {
		ws := m.selectedWorkspace()
		name := "this workspace"
		if ws != nil && ws.Name != "" {
			name = ws.Name
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			styleDialog.Render("Discard "+name+"? This deletes the worktree and branch. (y/n)"))
	}

	m.layout = ComputeLayout(m.width, m.height, m.diff.IsOpen())

	// Render panels.
	sidebarContent := m.sidebar.View(m.workspaces, m.selected,
		m.layout.Sidebar.ContentWidth(), m.layout.Sidebar.ContentHeight(), m.spinnerFrame)

	ws := m.selectedWorkspace()
	detailsContent := m.details.View(ws,
		m.layout.Details.ContentWidth(), m.layout.Details.ContentHeight(), m.spinnerFrame)

	// Build bordered panels.
	sidebarPanel := m.renderPanel("Workspaces", PanelWorkspaces, m.layout.Sidebar, sidebarContent)
	detailsPanel := m.renderPanel("Details", PanelDetails, m.layout.Details, detailsContent)

	// Left column: sidebar + details stacked.
	leftCol := lipgloss.JoinVertical(lipgloss.Left, sidebarPanel, detailsPanel)

	// Right side.
	var rightSide string
	if m.diff.IsOpen() && m.layout.Output.Width > 0 {
		// Split: output | diff
		outputPanel := m.renderOutputPanel(ws)
		diffPanel := m.renderDiffPanel()
		rightSide = lipgloss.JoinHorizontal(lipgloss.Top, outputPanel, diffPanel)
	} else if m.diff.IsOpen() {
		// Diff only (not enough room for split).
		rightSide = m.renderDiffPanel()
	} else {
		// Output only.
		rightSide = m.renderOutputPanel(ws)
	}

	// Combine left + right.
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightSide)

	// Status bar.
	statusBar := m.renderStatusBar()

	result := lipgloss.JoinVertical(lipgloss.Left, content, statusBar)

	return zones.Scan(result)
}

// renderPanel renders a bordered panel with title.
func (m *App) renderPanel(title string, id PanelID, rect Rect, content string) string {
	active := m.focus == id
	panel := buildPanel(title, active, rect.Width, rect.Height, content)
	return zones.Mark(panelZoneID(id), panel)
}

// renderOutputPanel renders the output panel (tabs + viewport + input).
func (m *App) renderOutputPanel(ws *agent.Workspace) string {
	rect := m.layout.Output
	if rect.Width <= 0 {
		return ""
	}

	active := m.focus == PanelOutput
	cw := rect.ContentWidth()
	ch := rect.ContentHeight()

	// Tabs — only shown for multi-session workspaces.
	tabs := ""
	if ws != nil && len(ws.Sessions) > 1 {
		tabs = m.output.ViewTabs(ws, cw)
		ch-- // tabs take one line
	}

	// Ensure viewport is sized correctly.
	if m.output.ready {
		if m.output.viewport.Width != cw || m.output.viewport.Height != ch {
			m.output.viewport.Width = cw
			m.output.viewport.Height = ch
		}
	}

	var content string
	if ws == nil || len(ws.Sessions) == 0 {
		content = lipgloss.Place(cw, ch, lipgloss.Center, lipgloss.Center,
			styleSubtle.Render("Press n to create a workspace"))
	} else {
		if tabs != "" {
			content = tabs + "\n" + m.output.ViewOutput()
		} else {
			content = m.output.ViewOutput()
		}
	}

	panel := buildPanel("Output", active, rect.Width, rect.Height, content)

	// Input bar below.
	inputRect := m.layout.Input
	var input string
	if inputRect.Width > 0 && inputRect.Height > 0 && ws != nil && !ws.Archived {
		input = m.output.ViewInput(inputRect.Width, active && m.output.input.Focused())
	}

	return zones.Mark(panelZoneID(PanelOutput), lipgloss.JoinVertical(lipgloss.Left, panel, input))
}

// renderDiffPanel renders the diff split panel.
func (m *App) renderDiffPanel() string {
	rect := m.layout.Diff
	if rect.Width <= 0 {
		return ""
	}

	active := m.focus == PanelDiff
	cw := rect.ContentWidth()
	ch := rect.ContentHeight()
	m.diff.Resize(cw, ch)

	content := m.diff.View()
	if content == "" {
		content = lipgloss.Place(cw, ch, lipgloss.Center, lipgloss.Center,
			styleSubtle.Render("Loading diff..."))
	}

	panel := buildPanel("Diff", active, rect.Width, rect.Height, content)
	return zones.Mark(panelZoneID(PanelDiff), panel)
}

// renderStatusBar renders the bottom status bar.
func (m *App) renderStatusBar() string {
	ws := m.selectedWorkspace()

	// Left: hints based on context.
	var hints []string
	if m.focus == PanelOutput && m.output.input.Focused() {
		hints = append(hints, "enter:send", "esc:blur")
	} else {
		hints = append(hints, "n:new", "d:diff", "m:merge", "p:pr", "x:discard")
		if ws != nil {
			s := ws.Status()
			if s == agent.StatusRunning {
				hints = append(hints, "c:cancel")
			}
		}
		hints = append(hints, "?:help", "q:quit")
	}

	var hintParts []string
	for _, h := range hints {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			hintParts = append(hintParts,
				styleStatusKey.Render(parts[0])+styleStatusBar.Render(":"+parts[1]))
		}
	}
	left := strings.Join(hintParts, styleStatusSep.Render("  "))

	// Right: agent count.
	right := styleSubtle.Render(fmt.Sprintf("%d/%d agents", m.pool.RunningCount(), m.cfg.General.MaxConcurrent))
	if m.repoName != "" {
		right = styleSubtle.Render(m.repoName+"  ") + right
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return " " + left + strings.Repeat(" ", gap) + right + " "
}

// --- Key handling ---

func (m *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Quit confirmation.
	if m.confirmQuit {
		switch msg.String() {
		case "y", "Y":
			m.pool.CancelAll()
			m.saveState()
			return m, tea.Quit
		default:
			m.confirmQuit = false
			return m, nil
		}
	}

	// Discard confirmation.
	if m.confirmDiscard {
		switch msg.String() {
		case "y", "Y":
			ws := m.selectedWorkspace()
			m.confirmDiscard = false
			if ws != nil {
				return m, m.discardCmd(ws)
			}
			return m, nil
		default:
			m.confirmDiscard = false
			return m, nil
		}
	}

	// Help overlay.
	if m.help.IsOpen() {
		if key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Escape) {
			m.help.Close()
		}
		return m, nil
	}

	// Dialog.
	if m.dialog.IsOpen() {
		cmd := m.dialog.Update(msg)
		if !m.dialog.IsOpen() && m.dialog.confirmed {
			agentName := m.dialog.agentName
			m.dialog.confirmed = false

			if m.dialog.mode == ModeAddAgent {
				targetWS := m.selectedWorkspace()
				if targetWS != nil {
					m.addSessionToWorkspace(targetWS, agentName)
					targetWS.ActiveIdx = len(targetWS.Sessions) - 1
					m.focusSelected()
					m.saveState()
				}
				return m, cmd
			}

			// New workspace.
			ws, _ := m.newWorkspace(agentName)
			m.workspaces = append(m.workspaces, ws)
			m.selected = len(m.workspaces) - 1
			m.focus = PanelOutput
			m.focusSelected()
			focusCmd := m.output.input.Focus()
			m.saveState()
			return m, tea.Batch(cmd, focusCmd)
		}
		return m, cmd
	}

	// Force quit always works.
	if key.Matches(msg, m.keys.ForceQuit) {
		m.pool.CancelAll()
		m.saveState()
		return m, tea.Quit
	}

	// When input is focused, most keys go to input.
	if m.focus == PanelOutput && m.output.input.Focused() {
		return m.handleInputKey(msg)
	}

	// Global keys.
	switch {
	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
		return m, nil

	case key.Matches(msg, m.keys.Quit):
		if m.pool.RunningCount() > 0 {
			m.confirmQuit = true
			return m, nil
		}
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Escape):
		if m.diff.IsOpen() {
			m.diff.Close()
			m.layout = ComputeLayout(m.width, m.height, false)
			return m, nil
		}
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.cyclePanel()
		return m, nil

	case key.Matches(msg, m.keys.Panel1):
		m.focus = PanelWorkspaces
		return m, nil

	case key.Matches(msg, m.keys.Panel2):
		m.focus = PanelDetails
		return m, nil

	case key.Matches(msg, m.keys.Panel3):
		m.focus = PanelOutput
		return m, nil
	}

	// Panel-specific keys.
	switch m.focus {
	case PanelWorkspaces, PanelDetails:
		return m.handleSidebarKey(msg)
	case PanelOutput:
		return m.handleOutputKey(msg)
	case PanelDiff:
		return m.handleDiffKey(msg)
	}

	return m, nil
}

func (m *App) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := m.selectedWorkspace()

	// Escape blurs the input.
	if key.Matches(msg, m.keys.Escape) {
		m.output.input.Blur()
		return m, nil
	}

	// Force quit.
	if key.Matches(msg, m.keys.ForceQuit) {
		m.pool.CancelAll()
		m.saveState()
		return m, tea.Quit
	}

	// Enter submits.
	if key.Matches(msg, m.keys.Enter) {
		if ws == nil {
			return m, nil
		}
		value := strings.TrimSpace(m.output.input.Value())
		if value == "" {
			return m, nil
		}
		sess := ws.ActiveSession()
		if sess == nil {
			return m, nil
		}
		if sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing {
			return m, nil
		}

		// Show user message.
		if sess.Output.Len() > 0 {
			sess.Output.Write("")
		}
		sess.Output.Write("\u276f " + value)
		sess.Output.Write("")
		m.output.input.SetValue("")

		if ws.WorktreePath == "" {
			// First message — create worktree and start.
			sess.Task = value
			sess.Status = agent.StatusInitializing
			m.output.input.Placeholder = "Send follow-up..."
			m.output.UpdateOutput(ws)
			return m, tea.Batch(
				m.setupAndStartCmd(ws, sess, value, sess.Agent.Name),
				m.ensureAnimating(),
			)
		}

		if sess.Task == "" {
			// First message for this session (worktree exists).
			sess.Task = value
			sess.Status = agent.StatusRunning
			m.output.input.Placeholder = "Send follow-up..."
			m.output.UpdateOutput(ws)
			return m, tea.Batch(m.startSessionCmd(ws, sess), m.ensureAnimating())
		}

		// Follow-up.
		sess.Status = agent.StatusRunning
		sess.TurnCount++
		m.output.UpdateOutput(ws)
		return m, tea.Batch(m.sendFollowUpCmd(ws, sess, value), m.ensureAnimating())
	}

	// Drop leaked mouse events during scroll.
	if !m.lastScrollAt.IsZero() && time.Since(m.lastScrollAt) < 100*time.Millisecond {
		return m, nil
	}

	// Pass to input.
	cmd := m.output.Update(msg, ws)
	return m, cmd
}

func (m *App) handleSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := m.selectedWorkspace()

	switch {
	case key.Matches(msg, m.keys.Up):
		if len(m.workspaces) > 0 {
			m.selected--
			if m.selected < 0 {
				m.selected = len(m.workspaces) - 1
			}
			m.focusSelected()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if len(m.workspaces) > 0 {
			m.selected++
			if m.selected >= len(m.workspaces) {
				m.selected = 0
			}
			m.focusSelected()
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		m.focus = PanelOutput
		if ws != nil && !ws.Archived {
			return m, m.output.input.Focus()
		}
		return m, nil

	case key.Matches(msg, m.keys.New):
		if len(m.cfg.Agents) <= 1 {
			ws, _ := m.newWorkspace(m.cfg.General.DefaultAgent)
			m.workspaces = append(m.workspaces, ws)
			m.selected = len(m.workspaces) - 1
			m.focus = PanelOutput
			m.focusSelected()
			focusCmd := m.output.input.Focus()
			m.saveState()
			return m, focusCmd
		}
		m.dialog.Open()
		return m, nil

	case key.Matches(msg, m.keys.Agent):
		if ws != nil && ws.WorktreePath != "" && !ws.Archived {
			m.dialog.OpenAddAgent()
		}
		return m, nil

	case key.Matches(msg, m.keys.Diff):
		if ws != nil && !ws.Archived && ws.WorktreePath != "" {
			if m.diff.IsOpen() {
				m.diff.Close()
				m.layout = ComputeLayout(m.width, m.height, false)
			} else {
				return m, m.loadDiffCmd(ws)
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Merge):
		if ws != nil && !ws.Archived {
			if ws.PRNumber > 0 {
				sess := ws.ActiveSession()
				if sess != nil {
					sess.Output.Write("")
					sess.Output.Write("\u29d6 Merging PR...")
					m.output.UpdateOutput(ws)
				}
				return m, m.mergePRCmd(ws)
			} else if ws.Status() == agent.StatusCompleted || ws.Status() == agent.StatusWaiting {
				return m, m.mergeCmd(ws)
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.PR):
		if ws != nil && ws.WorktreePath != "" && ws.PRNumber == 0 && !ws.Archived {
			sess := ws.ActiveSession()
			if sess != nil {
				sess.Output.Write("")
				sess.Output.Write("\u29d6 Creating PR...")
				m.output.UpdateOutput(ws)
			}
			return m, m.createPRCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.keys.Discard):
		if ws != nil && !ws.Archived {
			m.confirmDiscard = true
		}
		return m, nil

	case key.Matches(msg, m.keys.Cancel):
		if ws != nil {
			sess := ws.ActiveSession()
			if sess != nil && (sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing) {
				if sess.Cancel != nil {
					sess.Cancel()
				}
				sess.Status = agent.StatusCancelled
				m.output.UpdateOutput(ws)
				m.saveState()
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *App) handleOutputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := m.selectedWorkspace()

	switch {
	case key.Matches(msg, m.keys.Enter):
		if ws != nil && !ws.Archived {
			return m, m.output.input.Focus()
		}
		return m, nil

	case key.Matches(msg, m.keys.PageUp):
		m.output.PageUp()
		return m, nil

	case key.Matches(msg, m.keys.PageDown):
		m.output.PageDown()
		return m, nil

	case key.Matches(msg, m.keys.Top):
		m.output.GotoTop()
		return m, nil

	case key.Matches(msg, m.keys.Bottom):
		m.output.GotoBottom()
		return m, nil

	case key.Matches(msg, m.keys.NextTab):
		if ws != nil && len(ws.Sessions) > 1 {
			ws.ActiveIdx = (ws.ActiveIdx + 1) % len(ws.Sessions)
			m.focusSelected()
		}
		return m, nil

	case key.Matches(msg, m.keys.PrevTab):
		if ws != nil && len(ws.Sessions) > 1 {
			ws.ActiveIdx--
			if ws.ActiveIdx < 0 {
				ws.ActiveIdx = len(ws.Sessions) - 1
			}
			m.focusSelected()
		}
		return m, nil

	// Allow workspace actions from output panel too.
	case key.Matches(msg, m.keys.New):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Agent):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Diff):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Merge):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.PR):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Discard):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Cancel):
		return m.handleSidebarKey(msg)

	case key.Matches(msg, m.keys.Up):
		return m.handleSidebarKey(msg)
	case key.Matches(msg, m.keys.Down):
		return m.handleSidebarKey(msg)
	}

	return m, nil
}

func (m *App) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.PageUp):
		m.diff.PageUp()
	case key.Matches(msg, m.keys.PageDown):
		m.diff.PageDown()
	case key.Matches(msg, m.keys.Top):
		m.diff.GotoTop()
	case key.Matches(msg, m.keys.Bottom):
		m.diff.GotoBottom()

	case key.Matches(msg, m.keys.Merge):
		ws := m.findWorkspace(m.diff.workspaceID)
		if ws != nil {
			return m, m.mergeCmd(ws)
		}
	case key.Matches(msg, m.keys.Discard):
		ws := m.findWorkspace(m.diff.workspaceID)
		if ws != nil {
			m.confirmDiscard = true
		}
	}
	return m, nil
}

// cyclePanel cycles focus between panels.
func (m *App) cyclePanel() {
	if m.diff.IsOpen() {
		switch m.focus {
		case PanelWorkspaces:
			m.focus = PanelDetails
		case PanelDetails:
			m.focus = PanelOutput
		case PanelOutput:
			m.focus = PanelDiff
		case PanelDiff:
			m.focus = PanelWorkspaces
		}
	} else {
		switch m.focus {
		case PanelWorkspaces:
			m.focus = PanelDetails
		case PanelDetails:
			m.focus = PanelOutput
		default:
			m.focus = PanelWorkspaces
		}
	}
	// Blur input when leaving output panel.
	if m.focus != PanelOutput {
		m.output.input.Blur()
	}
}

// --- Mouse handling ---

func (m *App) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.confirmQuit || m.confirmDiscard || m.dialog.IsOpen() || m.help.IsOpen() {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			break
		}

		// Check if click is in the input bar area — focus input (only if visible).
		if m.isInRect(msg.X, msg.Y, m.layout.Input) {
			ws := m.selectedWorkspace()
			if ws != nil && !ws.Archived {
				m.focus = PanelOutput
				return m, m.output.input.Focus()
			}
		}

		// Check panel zones for focus.
		for _, pid := range []PanelID{PanelWorkspaces, PanelDetails, PanelOutput, PanelDiff} {
			if z := zones.Get(panelZoneID(pid)); z != nil && z.InBounds(msg) {
				oldFocus := m.focus
				m.focus = pid
				// Blur input when clicking away from output.
				if oldFocus == PanelOutput && pid != PanelOutput {
					m.output.input.Blur()
				}
				break
			}
		}

		// Check sidebar workspace zones.
		for i := range m.workspaces {
			if z := zones.Get(sidebarZoneID(i)); z != nil && z.InBounds(msg) {
				m.selected = i
				m.focusSelected()
				break
			}
		}

		// Check session tab zones.
		ws := m.selectedWorkspace()
		if ws != nil {
			for i := range ws.Sessions {
				if z := zones.Get(sessionTabZoneID(i)); z != nil && z.InBounds(msg) {
					ws.ActiveIdx = i
					m.focusSelected()
					break
				}
			}
		}

	case tea.MouseButtonWheelUp:
		m.lastScrollAt = time.Now()
		if m.focus == PanelOutput || m.isInRect(msg.X, msg.Y, m.layout.Output) {
			m.output.ScrollUp(3)
		} else if m.focus == PanelDiff || m.isInRect(msg.X, msg.Y, m.layout.Diff) {
			m.diff.ScrollUp(3)
		} else if m.isInRect(msg.X, msg.Y, m.layout.Sidebar) {
			if m.selected > 0 {
				m.selected--
				m.focusSelected()
			}
		}

	case tea.MouseButtonWheelDown:
		m.lastScrollAt = time.Now()
		if m.focus == PanelOutput || m.isInRect(msg.X, msg.Y, m.layout.Output) {
			m.output.ScrollDown(3)
		} else if m.focus == PanelDiff || m.isInRect(msg.X, msg.Y, m.layout.Diff) {
			m.diff.ScrollDown(3)
		} else if m.isInRect(msg.X, msg.Y, m.layout.Sidebar) {
			if m.selected < len(m.workspaces)-1 {
				m.selected++
				m.focusSelected()
			}
		}
	}

	return m, nil
}

func (m *App) isInRect(x, y int, r Rect) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

// --- Async message handlers ---

func (m *App) handleOutputMsg(msg agent.OutputMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	ws.UpdatedAt = time.Now()
	for _, sess := range ws.Sessions {
		if sess.ID == msg.SessionID {
			sess.UpdatedAt = time.Now()
			break
		}
	}

	sel := m.selectedWorkspace()
	if sel != nil && sel.ID == ws.ID {
		activeSess := ws.ActiveSession()
		if activeSess != nil && activeSess.ID == msg.SessionID {
			m.output.UpdateOutput(ws)
		}
	}

	return m, nil
}

func (m *App) handleDoneMsg(msg agent.DoneMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	for _, sess := range ws.Sessions {
		if sess.ID == msg.SessionID {
			sess.ExitCode = msg.ExitCode
			sess.UpdatedAt = time.Now()
			if msg.Err != nil {
				sess.Status = agent.StatusErrored
				sess.Error = msg.Err
			} else if msg.ExitCode == 0 {
				sess.Status = agent.StatusCompleted
			} else {
				sess.Status = agent.StatusErrored
			}
			break
		}
	}

	m.pool.Release()
	m.pool.Unregister(msg.SessionID)

	var focusCmd tea.Cmd
	sel := m.selectedWorkspace()
	if sel != nil && sel.ID == ws.ID {
		activeSess := ws.ActiveSession()
		if activeSess != nil && activeSess.ID == msg.SessionID {
			m.output.UpdateOutput(ws)
			focusCmd = m.output.input.Focus()
		}
	}

	m.saveState()

	return m, focusCmd
}

func (m *App) handleDiffReady(msg DiffReadyMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	cw := m.layout.Diff.ContentWidth()
	ch := m.layout.Diff.ContentHeight()
	if cw <= 0 {
		// Diff panel not yet computed — compute layout with diff open.
		m.layout = ComputeLayout(m.width, m.height, true)
		cw = m.layout.Diff.ContentWidth()
		ch = m.layout.Diff.ContentHeight()
	}
	m.diff.SetDiff(msg.WorkspaceID, msg.Diff, cw, ch)
	m.layout = ComputeLayout(m.width, m.height, true)
	return m, nil
}

func (m *App) handleMergeComplete(msg MergeCompleteMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if msg.Err != nil {
		if ws != nil {
			sess := ws.ActiveSession()
			if sess != nil {
				sess.Output.Write("\u2717 Merge failed: " + msg.Err.Error())
				m.output.UpdateOutput(ws)
			}
		}
		return m, nil
	}

	if ws != nil && ws.PRNumber > 0 {
		ws.Archived = true
		ws.WorktreePath = ""
		sess := ws.ActiveSession()
		if sess != nil {
			sess.Output.Write("")
			sess.Output.Write(fmt.Sprintf("\u2713 PR #%d merged and workspace archived", ws.PRNumber))
		}
	} else {
		m.removeWorkspace(msg.WorkspaceID)
	}

	m.diff.Close()
	m.layout = ComputeLayout(m.width, m.height, false)
	m.saveState()
	return m, nil
}

func (m *App) handleDiscardComplete(msg DiscardCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	_ = state.RemoveHistory(m.historyDir, msg.WorkspaceID)
	m.removeWorkspace(msg.WorkspaceID)
	m.diff.Close()
	m.layout = ComputeLayout(m.width, m.height, false)
	m.saveState()
	return m, nil
}

func (m *App) handleWorkspaceRename(msg WorkspaceRenameMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}
	ws.Name = msg.NewName
	ws.Branch = msg.NewBranch
	m.saveState()
	return m, nil
}

func (m *App) handlePRCreateComplete(msg PRCreateCompleteMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	if msg.Err != nil {
		sess := ws.ActiveSession()
		if sess != nil {
			sess.Output.Write("\u2717 PR creation failed: " + msg.Err.Error())
			m.output.UpdateOutput(ws)
		}
		return m, nil
	}

	ws.PRNumber = msg.PRNumber
	ws.PRURL = msg.PRURL

	sess := ws.ActiveSession()
	if sess != nil {
		sess.Output.Write(fmt.Sprintf("\u2713 PR #%d created: %s", msg.PRNumber, msg.PRURL))
		m.output.UpdateOutput(ws)
	}

	m.saveState()
	return m, nil
}

// --- Commands ---

func (m *App) newWorkspace(agentName string) (*agent.Workspace, *agent.Session) {
	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	sessID := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	wsName := agent.PickCityName()

	acfg := m.cfg.Agents[agentName]
	agentDef := agent.AgentDefFromConfig(agentName, acfg)

	bufSize := m.cfg.General.OutputBufferSize
	sess := &agent.Session{
		ID:        sessID,
		Agent:     agentDef,
		Status:    agent.StatusWaiting,
		Output:    agent.NewRingBuffer(bufSize),
		Stderr:    agent.NewRingBuffer(bufSize),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ws := &agent.Workspace{
		ID:        wsID,
		Name:      wsName,
		Sessions:  []*agent.Session{sess},
		ActiveIdx: 0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return ws, sess
}

func (m *App) setupAndStartCmd(ws *agent.Workspace, sess *agent.Session, task, agentName string) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	cityName := ws.Name
	ptyRows, ptyCols := m.ptySize()

	return func() tea.Msg {
		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		fail := func(err error) tea.Msg {
			sess.Output.Write("Error: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: err}
		}

		if err := m.pool.Acquire(acquireCtx); err != nil {
			sess.Output.Write("Error: acquire pool slot: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return nil
		}

		ctx := context.Background()

		branch, worktreePath, err := m.gitMgr.Create(ctx, m.cfg.General.BaseBranch, cityName)
		if err != nil {
			return fail(fmt.Errorf("creating worktree: %w", err))
		}

		ws.Branch = branch
		ws.WorktreePath = worktreePath

		hookEnv := hooks.Env{
			WorkspaceID:  wsID,
			Branch:       branch,
			Agent:        agentName,
			WorktreePath: worktreePath,
			Task:         task,
			RepoRoot:     m.repoRoot,
		}

		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreWorkspaceCreate, hookEnv, worktreePath); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("pre_workspace_create hook: %w", err))
		}

		agentDef := sess.Agent
		if err := agentDef.SetupMCPConfig(worktreePath, m.cfg.MCP); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("MCP config setup: %w", err))
		}

		if m.cfg.Init.Script != "" {
			if _, err := m.hookRunner.Run(ctx, m.cfg.Init.Script, hookEnv, worktreePath); err != nil {
				_ = m.gitMgr.Remove(ctx, worktreePath)
				return fail(fmt.Errorf("init script: %w", err))
			}
		}

		prompt := task

		sessCtx, sessCancel := context.WithCancel(ctx)
		sess.Cancel = sessCancel

		prog := m.program
		runner := agent.NewRunner(wsID, sess, agentDef, worktreePath, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		runner.PtyRows = ptyRows
		runner.PtyCols = ptyCols
		m.pool.Register(sessID, runner)

		sess.Status = agent.StatusRunning
		if err := runner.Start(sessCtx, prompt, false); err != nil {
			sessCancel()
			m.pool.Unregister(sessID)
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("starting agent: %w", err))
		}

		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostWorkspaceCreate, hookEnv, worktreePath)

		// Async rename: generate a descriptive name and rename the branch.
		gitMgr := m.gitMgr
		go func() {
			rCtx, rCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer rCancel()

			wsName, err := agent.GenerateWorkspaceName(rCtx, prompt)
			if err != nil {
				return
			}

			newBranch := rebuildBranchName(branch, wsName.Slug)
			if err := gitMgr.RenameBranch(rCtx, branch, newBranch); err != nil {
				return
			}

			if prog != nil {
				prog.Send(WorkspaceRenameMsg{
					WorkspaceID: wsID,
					NewName:     wsName.Display,
					NewBranch:   newBranch,
				})
			}
		}()

		return nil
	}
}

func (m *App) addSessionToWorkspace(ws *agent.Workspace, agentName string) *agent.Session {
	sessID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	acfg := m.cfg.Agents[agentName]
	agentDef := agent.AgentDefFromConfig(agentName, acfg)

	bufSize := m.cfg.General.OutputBufferSize
	sess := &agent.Session{
		ID:        sessID,
		Agent:     agentDef,
		Status:    agent.StatusWaiting,
		Output:    agent.NewRingBuffer(bufSize),
		Stderr:    agent.NewRingBuffer(bufSize),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ws.Sessions = append(ws.Sessions, sess)
	return sess
}

func (m *App) startSessionCmd(ws *agent.Workspace, sess *agent.Session) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	worktreePath := ws.WorktreePath
	task := sess.Task
	ptyRows, ptyCols := m.ptySize()

	return func() tea.Msg {
		fail := func(err error) tea.Msg {
			sess.Output.Write("Error: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: err}
		}

		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		if err := m.pool.Acquire(acquireCtx); err != nil {
			sess.Output.Write("Error: acquire pool slot: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return nil
		}

		agentDef := sess.Agent
		if err := agentDef.SetupMCPConfig(worktreePath, m.cfg.MCP); err != nil {
			return fail(fmt.Errorf("MCP config setup: %w", err))
		}

		ctx := context.Background()
		sessCtx, sessCancel := context.WithCancel(ctx)
		sess.Cancel = sessCancel

		prog := m.program
		runner := agent.NewRunner(wsID, sess, agentDef, worktreePath, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		runner.PtyRows = ptyRows
		runner.PtyCols = ptyCols
		m.pool.Register(sessID, runner)

		sess.Status = agent.StatusRunning
		if err := runner.Start(sessCtx, task, false); err != nil {
			sessCancel()
			m.pool.Unregister(sessID)
			return fail(fmt.Errorf("starting agent: %w", err))
		}

		return nil
	}
}

func (m *App) sendFollowUpCmd(ws *agent.Workspace, sess *agent.Session, prompt string) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	worktreePath := ws.WorktreePath
	ptyRows, ptyCols := m.ptySize()
	return func() tea.Msg {
		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		if err := m.pool.Acquire(acquireCtx); err != nil {
			sess.Output.Write("Error: " + err.Error())
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: fmt.Errorf("acquire pool slot: %w", err)}
		}

		sessCtx, cancel := context.WithCancel(context.Background())
		sess.Cancel = cancel
		sess.UpdatedAt = time.Now()

		prog := m.program
		runner := agent.NewRunner(wsID, sess, sess.Agent, worktreePath, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		runner.PtyRows = ptyRows
		runner.PtyCols = ptyCols
		m.pool.Register(sessID, runner)

		if err := runner.Start(sessCtx, prompt, true); err != nil {
			cancel()
			m.pool.Release()
			m.pool.Unregister(sessID)
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: fmt.Errorf("start follow-up: %w", err)}
		}

		return nil
	}
}

func (m *App) loadDiffCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	baseBranch := m.cfg.General.BaseBranch
	return func() tea.Msg {
		diff, err := m.gitMgr.Diff(context.Background(), baseBranch, branch)
		return DiffReadyMsg{WorkspaceID: wsID, Diff: diff, Err: err}
	}
}

func (m *App) mergeCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	worktreePath := ws.WorktreePath
	baseBranch := m.cfg.General.BaseBranch

	agentName := ""
	if sess := ws.ActiveSession(); sess != nil && sess.Agent != nil {
		agentName = sess.Agent.Name
	}

	hookEnv := hooks.Env{
		WorkspaceID:  wsID,
		Branch:       branch,
		Agent:        agentName,
		WorktreePath: worktreePath,
		RepoRoot:     m.repoRoot,
	}

	return func() tea.Msg {
		ctx := context.Background()

		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreMerge, hookEnv, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		if err := m.gitMgr.Remove(ctx, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		if err := m.gitMgr.Merge(ctx, branch, baseBranch); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostMerge, hookEnv, m.repoRoot)

		return MergeCompleteMsg{WorkspaceID: wsID}
	}
}

func (m *App) discardCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	worktreePath := ws.WorktreePath

	agentName := ""
	if sess := ws.ActiveSession(); sess != nil && sess.Agent != nil {
		agentName = sess.Agent.Name
	}

	hookEnv := hooks.Env{
		WorkspaceID:  wsID,
		Branch:       branch,
		Agent:        agentName,
		WorktreePath: worktreePath,
		RepoRoot:     m.repoRoot,
	}

	for _, sess := range ws.Sessions {
		if sess.Cancel != nil {
			sess.Cancel()
		}
	}

	return func() tea.Msg {
		ctx := context.Background()
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PreDiscard, hookEnv, worktreePath)

		if branch != "" && worktreePath != "" {
			if err := m.gitMgr.Discard(ctx, branch, worktreePath); err != nil {
				return DiscardCompleteMsg{WorkspaceID: wsID, Err: err}
			}
		}

		return DiscardCompleteMsg{WorkspaceID: wsID}
	}
}

func (m *App) createPRCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	baseBranch := m.cfg.General.BaseBranch
	wsName := ws.Name

	return func() tea.Msg {
		ctx := context.Background()

		if err := m.gitMgr.Push(ctx, branch); err != nil {
			return PRCreateCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		title, body, err := m.gitMgr.GeneratePRDescription(ctx, baseBranch, branch)
		if err != nil || title == "" {
			title = wsName
		}

		pr, err := m.gitMgr.CreatePR(ctx, branch, baseBranch, title, body)
		if err != nil {
			return PRCreateCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		return PRCreateCompleteMsg{
			WorkspaceID: wsID,
			PRNumber:    pr.Number,
			PRURL:       pr.URL,
		}
	}
}

func (m *App) mergePRCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	prNumber := ws.PRNumber
	worktreePath := ws.WorktreePath
	baseBranch := m.cfg.General.BaseBranch

	agentName := ""
	if sess := ws.ActiveSession(); sess != nil && sess.Agent != nil {
		agentName = sess.Agent.Name
	}

	hookEnv := hooks.Env{
		WorkspaceID:  wsID,
		Branch:       ws.Branch,
		Agent:        agentName,
		WorktreePath: worktreePath,
		RepoRoot:     m.repoRoot,
	}

	return func() tea.Msg {
		ctx := context.Background()

		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreMerge, hookEnv, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		if err := m.gitMgr.MergePR(ctx, prNumber); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		if worktreePath != "" {
			_ = m.gitMgr.Remove(ctx, worktreePath)
		}

		_ = m.gitMgr.PullBase(ctx, baseBranch)

		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostMerge, hookEnv, m.repoRoot)

		return MergeCompleteMsg{WorkspaceID: wsID}
	}
}

// --- Animation ---

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

func animTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

func (m *App) handleAnimTick() (tea.Model, tea.Cmd) {
	m.spinnerFrame++
	m.output.spinnerFrame = m.spinnerFrame

	sel := m.selectedWorkspace()
	if sel != nil {
		sess := sel.ActiveSession()
		if sess != nil && (sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing) {
			m.output.UpdateOutput(sel)
		}
	}

	if m.hasActiveSession() {
		return m, animTickCmd()
	}
	m.animating = false
	return m, nil
}

func (m *App) hasActiveSession() bool {
	for _, ws := range m.workspaces {
		for _, sess := range ws.Sessions {
			if sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing {
				return true
			}
		}
	}
	return false
}

func (m *App) ensureAnimating() tea.Cmd {
	if m.animating {
		return nil
	}
	m.animating = true
	return animTickCmd()
}

// --- Helpers ---

func (m *App) findWorkspace(id string) *agent.Workspace {
	for _, ws := range m.workspaces {
		if ws.ID == id {
			return ws
		}
	}
	return nil
}

func (m *App) removeWorkspace(id string) {
	for i, ws := range m.workspaces {
		if ws.ID == id {
			m.workspaces = append(m.workspaces[:i], m.workspaces[i+1:]...)
			if m.selected >= len(m.workspaces) && len(m.workspaces) > 0 {
				m.selected = len(m.workspaces) - 1
			}
			return
		}
	}
}

func (m *App) selectedWorkspace() *agent.Workspace {
	if len(m.workspaces) == 0 {
		return nil
	}
	if m.selected < 0 || m.selected >= len(m.workspaces) {
		return nil
	}
	return m.workspaces[m.selected]
}

func (m *App) focusSelected() {
	ws := m.selectedWorkspace()
	if ws != nil {
		cw := m.layout.Output.ContentWidth()
		ch := m.layout.Output.ContentHeight()
		if ch > 1 {
			ch-- // account for tabs
		}
		m.output.Focus(ws, cw, ch)
	}
}

// --- State persistence ---

func (m *App) saveState() {
	s := m.buildState()
	if err := state.Save(m.statePath, s); err != nil {
		return
	}

	for _, ws := range m.workspaces {
		for _, sess := range ws.Sessions {
			if sess.Output == nil {
				continue
			}
			lines := sess.Output.Lines()
			if len(lines) > 0 {
				_ = state.SaveHistory(m.historyDir, ws.ID, sess.ID, lines)
			}
		}
	}
}

func (m *App) buildState() *state.IslandState {
	s := &state.IslandState{
		SelectedWorkspace: m.selected,
	}

	for _, ws := range m.workspaces {
		wss := state.WorkspaceState{
			ID:               ws.ID,
			Name:             ws.Name,
			Branch:           ws.Branch,
			WorktreePath:     ws.WorktreePath,
			PRNumber:         ws.PRNumber,
			PRURL:            ws.PRURL,
			Archived:         ws.Archived,
			ActiveSessionIdx: ws.ActiveIdx,
			CreatedAt:        ws.CreatedAt,
			UpdatedAt:        ws.UpdatedAt,
		}

		for _, sess := range ws.Sessions {
			agentName := ""
			if sess.Agent != nil {
				agentName = sess.Agent.Name
			}
			ss := state.SessionState{
				ID:        sess.ID,
				AgentName: agentName,
				Task:      sess.Task,
				Status:    sess.Status.String(),
				TurnCount: sess.TurnCount,
				ExitCode:  sess.ExitCode,
				CreatedAt: sess.CreatedAt,
				UpdatedAt: sess.UpdatedAt,
			}
			wss.Sessions = append(wss.Sessions, ss)
		}

		s.Workspaces = append(s.Workspaces, wss)
	}

	return s
}

func (m *App) restoreState(s *state.IslandState) {
	for _, wss := range s.Workspaces {
		if wss.WorktreePath != "" && !wss.Archived {
			if _, err := os.Stat(wss.WorktreePath); os.IsNotExist(err) {
				continue
			}
		}

		ws := &agent.Workspace{
			ID:           wss.ID,
			Name:         wss.Name,
			Branch:       wss.Branch,
			WorktreePath: wss.WorktreePath,
			PRNumber:     wss.PRNumber,
			PRURL:        wss.PRURL,
			Archived:     wss.Archived,
			ActiveIdx:    wss.ActiveSessionIdx,
			CreatedAt:    wss.CreatedAt,
			UpdatedAt:    wss.UpdatedAt,
		}

		for _, ss := range wss.Sessions {
			acfg, ok := m.cfg.Agents[ss.AgentName]
			if !ok {
				continue
			}
			agentDef := agent.AgentDefFromConfig(ss.AgentName, acfg)

			bufSize := m.cfg.General.OutputBufferSize
			sess := &agent.Session{
				ID:        ss.ID,
				Agent:     agentDef,
				Task:      ss.Task,
				Status:    agent.ParseStatus(ss.Status),
				TurnCount: ss.TurnCount,
				ExitCode:  ss.ExitCode,
				Output:    agent.NewRingBuffer(bufSize),
				Stderr:    agent.NewRingBuffer(bufSize),
				CreatedAt: ss.CreatedAt,
				UpdatedAt: ss.UpdatedAt,
			}

			lines, err := state.LoadHistory(m.historyDir, wss.ID, ss.ID)
			if err == nil && len(lines) > 0 {
				for _, line := range lines {
					sess.Output.Write(line)
				}
			}

			if sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing {
				sess.Status = agent.StatusWaiting
			}

			ws.Sessions = append(ws.Sessions, sess)
		}

		m.workspaces = append(m.workspaces, ws)
	}

	m.selected = s.SelectedWorkspace
	if m.selected >= len(m.workspaces) && len(m.workspaces) > 0 {
		m.selected = len(m.workspaces) - 1
	}
}

// ptySize returns the PTY dimensions based on the current output panel size.
func (m *App) ptySize() (rows, cols uint16) {
	cw := m.layout.Output.ContentWidth()
	ch := m.layout.Output.ContentHeight()
	if cw < 40 {
		cw = 80
	}
	if ch < 10 {
		ch = 40
	}
	return uint16(ch), uint16(cw)
}

func rebuildBranchName(oldBranch, newSlug string) string {
	const prefix = "island/"
	if !strings.HasPrefix(oldBranch, prefix) {
		return oldBranch
	}
	rest := oldBranch[len(prefix):]
	dashIdx := strings.Index(rest, "-")
	if dashIdx < 0 {
		return oldBranch
	}
	return prefix + rest[:dashIdx] + "-" + newSlug
}
