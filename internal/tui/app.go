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

// Screen represents which top-level screen is currently displayed.
type Screen int

const (
	ScreenMain Screen = iota // sidebar + workspace view
	ScreenDiff               // full-screen diff review
)

// Custom message types for async operations.

// InitOutputMsg carries a line of output from the init script.
type InitOutputMsg struct {
	WorkspaceID string
	Line        string
}

// DiffReadyMsg is sent when a diff has been loaded.
type DiffReadyMsg struct {
	WorkspaceID string
	Diff        string
	Err         error
}

// MergeCompleteMsg is sent when a merge operation completes.
type MergeCompleteMsg struct {
	WorkspaceID string
	Err         error
}

// DiscardCompleteMsg is sent when a discard operation completes.
type DiscardCompleteMsg struct {
	WorkspaceID string
	Err         error
}

// TickMsg is sent every second to update elapsed time displays.
type TickMsg struct{}

// AnimTickMsg drives the spinner animation at ~150ms intervals.
type AnimTickMsg struct{}

// WorkspaceRenameMsg is sent when the async Haiku naming call completes.
type WorkspaceRenameMsg struct {
	WorkspaceID string
	NewName     string
	NewBranch   string
	Err         error
}

// PRCreateCompleteMsg is sent when a PR has been created (or failed).
type PRCreateCompleteMsg struct {
	WorkspaceID string
	PRNumber    int
	PRURL       string
	Err         error
}

// App is the root Bubble Tea model that owns all state and routes to
// sub-views. The main screen shows a left sidebar with the workspace list and
// a right panel with the active workspace's output.
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
	statePath  string // .island/state.json
	historyDir string // .island/history/

	// State
	screen     Screen
	workspaces []*agent.Workspace
	selected   int // selected workspace index in sidebar

	// Sub-models
	sidebar  SidebarModel
	wsView   WorkspaceViewModel
	dialog   DialogModel
	diffView DiffViewModel

	// Window
	width  int
	height int

	// Animation
	spinnerFrame int
	animating    bool

	// Confirmation prompts
	confirmQuit    bool
	confirmDiscard bool
}

// NewApp creates and initializes the root TUI model.
// It restores persisted state from .island/ if available.
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
		screen:     ScreenMain,
		sidebar:    newSidebarModel(),
		wsView:     newWorkspaceViewModel(),
		dialog:     newDialogModel(cfg),
		diffView:   newDiffViewModel(),
	}

	// Restore persisted state.
	if s, err := state.Load(app.statePath); err == nil && s != nil {
		app.restoreState(s)
	}

	return app
}

// SetProgram stores a reference to the tea.Program so that async goroutines
// can send messages back.
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

	// Propagate to the active sub-model.
	return m.propagateToScreen(msg)
}

// View implements tea.Model.
func (m *App) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Dialog overlay takes over the entire screen.
	if m.dialog.IsOpen() {
		return m.dialog.View(m.width, m.height)
	}

	// Quit confirmation overlay.
	if m.confirmQuit {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			dialogStyle.Render("Agents are still running. Quit anyway? (y/n)"),
		)
	}

	// Discard confirmation overlay.
	if m.confirmDiscard {
		ws := m.selectedWorkspace()
		name := "this workspace"
		if ws != nil && ws.Name != "" {
			name = ws.Name
		}
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			dialogStyle.Render("Discard "+name+"? This will delete the worktree and branch. (y/n)"),
		)
	}

	var b strings.Builder

	// Header.
	header := m.renderHeader()
	b.WriteString(header)
	b.WriteByte('\n')

	// Content area = total height - header (1) - footer (1).
	contentHeight := m.height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	if m.screen == ScreenDiff {
		b.WriteString(m.diffView.View(m.width, contentHeight))
		b.WriteByte('\n')
		b.WriteString(m.renderFooter())
		return b.String()
	}

	// Main screen: sidebar + workspace view.
	sidebarWidth := 22
	mainWidth := m.width - sidebarWidth
	if mainWidth < 20 {
		mainWidth = 20
		sidebarWidth = m.width - mainWidth
	}

	sidebar := m.sidebar.View(m.workspaces, m.selected, sidebarWidth, contentHeight, m.spinnerFrame)

	var mainContent string
	if len(m.workspaces) == 0 {
		mainContent = lipgloss.Place(mainWidth, contentHeight,
			lipgloss.Center, lipgloss.Center,
			footerStyle.Render("Press n to create a workspace"))
	} else {
		ws := m.selectedWorkspace()
		if ws != nil {
			// Make sure the workspace view is focused on the selected workspace.
			if m.wsView.lastWSID != ws.ID {
				m.wsView.Focus(ws, mainWidth, contentHeight)
			}
			mainContent = m.wsView.View(ws, mainWidth, contentHeight)
		} else {
			mainContent = lipgloss.Place(mainWidth, contentHeight,
				lipgloss.Center, lipgloss.Center,
				footerStyle.Render("No workspace selected"))
		}
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainContent)
	b.WriteString(content)
	b.WriteByte('\n')

	// Footer.
	b.WriteString(m.renderFooter())

	return b.String()
}

// --- Key handling ---

func (m *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Quit confirmation takes priority.
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

	// Discard confirmation takes priority.
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

	// Dialog takes priority when open.
	if m.dialog.IsOpen() {
		cmd := m.dialog.Update(msg)
		// Check if dialog just closed with a confirmed result.
		if !m.dialog.IsOpen() && m.dialog.confirmed {
			agentName := m.dialog.agentName
			m.dialog.confirmed = false

			if m.dialog.mode == ModeAddAgent {
				// Add a new agent session to the selected workspace.
				// The session starts in waiting state — the user must
				// send a prompt before the agent starts.
				targetWS := m.selectedWorkspace()
				if targetWS != nil {
					m.addSessionToWorkspace(targetWS, agentName)
					targetWS.ActiveIdx = len(targetWS.Sessions) - 1
					m.focusSelected()
					m.saveState()
				}
				return m, cmd
			}

			// New workspace mode — no agent starts yet. The user sends the
			// first message in the workspace input bar, which triggers
			// worktree creation and agent start.
			templateName := m.dialog.templateName
			ws, _ := m.newWorkspace(agentName)
			ws.TemplateName = templateName
			m.workspaces = append(m.workspaces, ws)
			m.selected = len(m.workspaces) - 1
			m.focusSelected()
			m.saveState()
			return m, cmd
		}
		return m, cmd
	}

	// Route to current screen.
	switch m.screen {
	case ScreenMain:
		return m.handleMainKey(msg)
	case ScreenDiff:
		return m.handleDiffKey(msg)
	}

	return m, nil
}

func (m *App) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := m.selectedWorkspace()

	// Tab/Shift+Tab always switch sessions.
	if ws != nil && len(ws.Sessions) > 1 {
		if key.Matches(msg, m.sidebar.keys.NextSession) {
			ws.ActiveIdx = (ws.ActiveIdx + 1) % len(ws.Sessions)
			m.focusSelected()
			return m, nil
		}
		if key.Matches(msg, m.sidebar.keys.PrevSession) {
			ws.ActiveIdx--
			if ws.ActiveIdx < 0 {
				ws.ActiveIdx = len(ws.Sessions) - 1
			}
			m.focusSelected()
			return m, nil
		}
	}

	// Ctrl-combo hotkeys take priority over the text input so they work
	// regardless of whether the input bar is focused.
	// NOTE: Ctrl+M is excluded here because terminals send the same byte
	// for Ctrl+M and Enter — it is handled below after the Enter/input logic.
	switch {
	case key.Matches(msg, m.sidebar.keys.Quit):
		if m.pool.RunningCount() > 0 {
			m.confirmQuit = true
			return m, nil
		}
		m.saveState()
		return m, tea.Quit

	case key.Matches(msg, m.sidebar.keys.New):
		// If only one agent and no templates, skip the dialog entirely.
		if len(m.cfg.Agents) <= 1 && len(m.cfg.Templates) == 0 {
			ws, _ := m.newWorkspace(m.cfg.General.DefaultAgent)
			m.workspaces = append(m.workspaces, ws)
			m.selected = len(m.workspaces) - 1
			m.focusSelected()
			m.saveState()
			return m, nil
		}
		m.dialog.Open()
		return m, nil

	case key.Matches(msg, m.sidebar.keys.AddAgent):
		if ws != nil && ws.WorktreePath != "" && !ws.Archived {
			m.dialog.OpenAddAgent()
		}
		return m, nil

	case key.Matches(msg, m.sidebar.keys.Diff):
		if ws != nil && !ws.Archived {
			return m, m.loadDiffCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.sidebar.keys.CreatePR):
		if ws != nil && ws.WorktreePath != "" && ws.PRNumber == 0 && !ws.Archived {
			sess := ws.ActiveSession()
			if sess != nil {
				sess.Output.Write("")
				sess.Output.Write(thinkingStyle.Render("Creating PR..."))
				m.wsView.UpdateOutput(ws)
			}
			return m, m.createPRCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.sidebar.keys.Discard):
		if ws != nil && !ws.Archived {
			m.confirmDiscard = true
		}
		return m, nil
	}

	// Archived workspaces are read-only — no input handling.
	if ws != nil && ws.Archived {
		// Only allow navigation keys for archived workspaces.
		switch {
		case key.Matches(msg, m.sidebar.keys.Up):
			if len(m.workspaces) > 0 {
				m.selected--
				if m.selected < 0 {
					m.selected = len(m.workspaces) - 1
				}
				m.focusSelected()
			}
			return m, nil
		case key.Matches(msg, m.sidebar.keys.Down):
			if len(m.workspaces) > 0 {
				m.selected++
				if m.selected >= len(m.workspaces) {
					m.selected = 0
				}
				m.focusSelected()
			}
			return m, nil
		}
		// Pass navigation keys to workspace view.
		cmd := m.wsView.Update(msg, ws)
		return m, cmd
	}

	// Input-aware handling for the active workspace.
	if ws != nil {
		sess := ws.ActiveSession()

		if msg.String() == "esc" {
			if m.wsView.input.Focused() {
				m.wsView.input.Blur()
			}
			return m, nil
		}

		// Enter: send message when input is focused, otherwise re-focus.
		if msg.String() == "enter" {
			if m.wsView.input.Focused() {
				value := strings.TrimSpace(m.wsView.input.Value())
				if value != "" && sess != nil {
					if sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing {
						// Agent is busy — ignore.
						return m, nil
					}

					// Show user message immediately in the output.
					if sess.Output.Len() > 0 {
						sess.Output.Write("") // separator for follow-ups
					}
					sess.Output.Write(userMsgStyle.Render("❯ " + value))
					sess.Output.Write("")
					m.wsView.input.SetValue("")

					if ws.WorktreePath == "" {
						// First message in workspace — create worktree and start agent.
						sess.Task = value
						sess.Status = agent.StatusInitializing
						m.wsView.input.Placeholder = "Send follow-up..."
						m.wsView.UpdateOutput(ws)

						agentName := ""
						if sess.Agent != nil {
							agentName = sess.Agent.Name
						}
						return m, tea.Batch(
							m.setupAndStartCmd(ws, sess, value, agentName, ws.TemplateName),
							m.ensureAnimating(),
						)
					}

					if sess.Task == "" {
						// First message for this session (worktree already exists).
						sess.Task = value
						sess.Status = agent.StatusRunning
						m.wsView.input.Placeholder = "Send follow-up..."
						m.wsView.UpdateOutput(ws)
						return m, tea.Batch(m.startSessionCmd(ws, sess), m.ensureAnimating())
					}

					// Follow-up message to an existing session.
					sess.Status = agent.StatusRunning
					sess.TurnCount++
					m.wsView.UpdateOutput(ws)
					return m, tea.Batch(m.sendFollowUpCmd(ws, sess, value), m.ensureAnimating())
				}
				return m, nil
			}
			// Input blurred — re-focus it.
			m.wsView.input.Focus()
			return m, nil
		}

		// When input is focused, remaining keys go to the text input.
		if m.wsView.input.Focused() {
			cmd := m.wsView.Update(msg, ws)
			return m, cmd
		}

		// Input is blurred — handle workspace-level keys before
		// the catch-all that re-focuses the input.
		if key.Matches(msg, m.wsView.keys.Cancel) {
			if sess != nil && (sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing) {
				if sess.Cancel != nil {
					sess.Cancel()
				}
				sess.Status = agent.StatusCancelled
				m.wsView.UpdateOutput(ws)
				m.saveState()
			}
			return m, nil
		}

		// Any other printable key re-focuses and types.
		k := msg.String()
		if len(k) == 1 && k[0] >= 32 && k[0] < 127 {
			m.wsView.input.Focus()
			cmd := m.wsView.Update(msg, ws)
			return m, cmd
		}
	}

	// Hotkeys that only apply when the input is blurred. This includes
	// Ctrl+M (merge) which shares a key code with Enter.
	switch {
	case key.Matches(msg, m.sidebar.keys.Merge):
		if ws != nil && !ws.Archived {
			if ws.PRNumber > 0 {
				// Merge the GitHub PR.
				sess := ws.ActiveSession()
				if sess != nil {
					sess.Output.Write("")
					sess.Output.Write(thinkingStyle.Render("Merging PR..."))
					m.wsView.UpdateOutput(ws)
				}
				return m, m.mergePRCmd(ws)
			} else if ws.Status() == agent.StatusCompleted || ws.Status() == agent.StatusWaiting {
				// Local merge (no PR).
				return m, m.mergeCmd(ws)
			}
		}
		return m, nil

	case key.Matches(msg, m.sidebar.keys.Up):
		if len(m.workspaces) > 0 {
			m.selected--
			if m.selected < 0 {
				m.selected = len(m.workspaces) - 1
			}
			m.focusSelected()
		}
		return m, nil

	case key.Matches(msg, m.sidebar.keys.Down):
		if len(m.workspaces) > 0 {
			m.selected++
			if m.selected >= len(m.workspaces) {
				m.selected = 0
			}
			m.focusSelected()
		}
		return m, nil
	}

	// Pass remaining keys to the workspace view.
	if ws != nil {
		cmd := m.wsView.Update(msg, ws)
		return m, cmd
	}

	return m, nil
}

func (m *App) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.diffView.keys.Back):
		m.screen = ScreenMain
		return m, nil

	case key.Matches(msg, m.diffView.keys.Merge):
		ws := m.findWorkspace(m.diffView.workspaceID)
		if ws != nil {
			return m, m.mergeCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.diffView.keys.Discard):
		ws := m.findWorkspace(m.diffView.workspaceID)
		if ws != nil {
			m.confirmDiscard = true
			m.screen = ScreenMain
		}
		return m, nil
	}

	cmd := m.diffView.Update(msg)
	return m, cmd
}

// --- Mouse handling ---

func (m *App) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.screen != ScreenMain {
		// In diff view, pass mouse events to the diff viewport.
		cmd := m.diffView.Update(msg)
		return m, cmd
	}

	sidebarWidth := 22

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.X < sidebarWidth {
			// Click in sidebar: select workspace.
			// Account for header (1 line) + sidebar header + padding.
			row := msg.Y - 3
			if row >= 0 && row < len(m.workspaces) {
				m.selected = row
				m.focusSelected()
			}
		}

	case tea.MouseButtonWheelUp:
		if msg.X >= sidebarWidth {
			// Scroll up in workspace viewport.
			m.wsView.autoScroll = false
			m.wsView.viewport.LineUp(3)
		}

	case tea.MouseButtonWheelDown:
		if msg.X >= sidebarWidth {
			// Scroll down in workspace viewport.
			m.wsView.viewport.LineDown(3)
			if m.wsView.viewport.AtBottom() {
				m.wsView.autoScroll = true
			}
		}
	}

	return m, nil
}

// --- Async message handlers ---

func (m *App) handleOutputMsg(msg agent.OutputMsg) (tea.Model, tea.Cmd) {
	// Find the workspace by WorkspaceID.
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	// The runner already writes to the ring buffer. The TUI handler does NOT
	// write to it -- it just refreshes the viewport from the ring buffer.
	ws.UpdatedAt = time.Now()

	// Find the session and update its timestamp.
	for _, sess := range ws.Sessions {
		if sess.ID == msg.SessionID {
			sess.UpdatedAt = time.Now()
			break
		}
	}

	// If we are viewing this workspace's active session, refresh the viewport.
	if m.screen == ScreenMain {
		sel := m.selectedWorkspace()
		if sel != nil && sel.ID == ws.ID {
			activeSess := ws.ActiveSession()
			if activeSess != nil && activeSess.ID == msg.SessionID {
				m.wsView.UpdateOutput(ws)
			}
		}
	}

	return m, nil
}

func (m *App) handleDoneMsg(msg agent.DoneMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	// Find the session and update its status.
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

	// If this is the active session of the selected workspace, refresh the
	// viewport to show the completion indicator and focus the input.
	sel := m.selectedWorkspace()
	if sel != nil && sel.ID == ws.ID {
		activeSess := ws.ActiveSession()
		if activeSess != nil && activeSess.ID == msg.SessionID {
			m.wsView.UpdateOutput(ws)
			m.wsView.input.Focus()
		}
	}

	m.saveState()
	return m, nil
}

func (m *App) handleDiffReady(msg DiffReadyMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	ws := m.findWorkspace(msg.WorkspaceID)
	wsName := ""
	if ws != nil {
		wsName = ws.Name
	}
	m.diffView.SetDiff(msg.WorkspaceID, wsName, msg.Diff, m.width, m.height-2)
	m.screen = ScreenDiff
	return m, nil
}

func (m *App) handleMergeComplete(msg MergeCompleteMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if msg.Err != nil {
		// Show error to user.
		if ws != nil {
			sess := ws.ActiveSession()
			if sess != nil {
				sess.Output.Write(erroredStyle.Render("Merge failed: " + msg.Err.Error()))
				m.wsView.UpdateOutput(ws)
			}
		}
		return m, nil
	}

	if ws != nil && ws.PRNumber > 0 {
		// PR merge: archive the workspace.
		ws.Archived = true
		ws.WorktreePath = ""
		sess := ws.ActiveSession()
		if sess != nil {
			sess.Output.Write("")
			sess.Output.Write(completedStyle.Render(fmt.Sprintf("\u2713 PR #%d merged and workspace archived", ws.PRNumber)))
		}
	} else {
		// Local merge: remove workspace entirely.
		m.removeWorkspace(msg.WorkspaceID)
	}

	m.screen = ScreenMain
	m.saveState()
	return m, nil
}

func (m *App) handleDiscardComplete(msg DiscardCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	// Remove history files for the discarded workspace.
	_ = state.RemoveHistory(m.historyDir, msg.WorkspaceID)
	m.removeWorkspace(msg.WorkspaceID)
	m.screen = ScreenMain
	m.saveState()
	return m, nil
}

// --- Propagate to sub-models ---

func (m *App) propagateToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenMain:
		ws := m.selectedWorkspace()
		if ws != nil {
			cmd := m.wsView.Update(msg, ws)
			return m, cmd
		}
	case ScreenDiff:
		cmd := m.diffView.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Commands ---

// newWorkspace creates a Workspace and Session in memory (no I/O). The
// workspace is added to m.workspaces by the caller so that it exists before
// any async runner messages arrive, eliminating the race where OutputMsg /
// DoneMsg are silently dropped because findWorkspace returns nil.
func (m *App) newWorkspace(agentName string) (*agent.Workspace, *agent.Session) {
	wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())
	sessID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	// Use a random city name as a placeholder until Haiku generates a
	// descriptive name from the first prompt.
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

// setupAndStartCmd returns a tea.Cmd that creates the git worktree, runs
// hooks, and starts the agent process. The workspace and session already
// exist in m.workspaces (added by the caller), so any OutputMsg / DoneMsg
// sent by the runner will be found by findWorkspace. Errors are written to
// the session's output buffer so the user can see them.
func (m *App) setupAndStartCmd(ws *agent.Workspace, sess *agent.Session, task, agentName, templateName string) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	cityName := ws.Name // capture city name for branch slug

	return func() tea.Msg {
		// Use a timeout for pool acquisition so we never block forever.
		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		// fail sends a DoneMsg so the TUI properly transitions out of the
		// initializing state. The caller must NOT release the pool slot —
		// handleDoneMsg does that.
		fail := func(err error) tea.Msg {
			sess.Output.Write("Error: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: err}
		}

		// Acquire pool slot.
		if err := m.pool.Acquire(acquireCtx); err != nil {
			sess.Output.Write("Error: acquire pool slot: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			// No slot acquired — return nil so handleDoneMsg does not
			// try to release a slot we never held.
			return nil
		}

		// From here on, a pool slot is held. On error we return a DoneMsg
		// and let handleDoneMsg release the slot.

		// Use city name for the initial branch (renamed async by Haiku).
		ctx := context.Background()

		// Create git worktree.
		branch, worktreePath, err := m.gitMgr.Create(ctx, m.cfg.General.BaseBranch, cityName)
		if err != nil {
			return fail(fmt.Errorf("creating worktree: %w", err))
		}

		ws.Branch = branch
		ws.WorktreePath = worktreePath

		// Build hook env.
		hookEnv := hooks.Env{
			WorkspaceID:  wsID,
			Branch:       branch,
			Agent:        agentName,
			WorktreePath: worktreePath,
			Task:         task,
			RepoRoot:     m.repoRoot,
		}

		// Run pre_workspace_create hook.
		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreWorkspaceCreate, hookEnv, worktreePath); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("pre_workspace_create hook: %w", err))
		}

		// Set up MCP config.
		agentDef := sess.Agent
		if err := agentDef.SetupMCPConfig(worktreePath, m.cfg.MCP); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("MCP config setup: %w", err))
		}

		// Run init script.
		if m.cfg.Init.Script != "" {
			if _, err := m.hookRunner.Run(ctx, m.cfg.Init.Script, hookEnv, worktreePath); err != nil {
				_ = m.gitMgr.Remove(ctx, worktreePath)
				return fail(fmt.Errorf("init script: %w", err))
			}
		}

		// Resolve prompt.
		prompt := task
		if templateName != "" {
			if tmpl, ok := m.cfg.Templates[templateName]; ok {
				prompt = config.ApplyTemplate(tmpl, task)
			}
		}

		// Create runner.
		sessCtx, sessCancel := context.WithCancel(ctx)
		sess.Cancel = sessCancel

		prog := m.program
		runner := agent.NewRunner(wsID, sess, agentDef, worktreePath, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		m.pool.Register(sessID, runner)

		// Start the agent.
		sess.Status = agent.StatusRunning
		if err := runner.Start(sessCtx, prompt, false); err != nil {
			sessCancel()
			m.pool.Unregister(sessID)
			_ = m.gitMgr.Remove(ctx, worktreePath)
			return fail(fmt.Errorf("starting agent: %w", err))
		}

		// Run post_workspace_create hook.
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostWorkspaceCreate, hookEnv, worktreePath)

		// Async: call Haiku to generate a descriptive name and rename the branch.
		gitMgr := m.gitMgr
		go func() {
			rCtx, rCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer rCancel()

			name, err := agent.GenerateBranchName(rCtx, prompt)
			if err != nil {
				return // keep city name
			}

			newBranch := rebuildBranchName(branch, name)
			if err := gitMgr.RenameBranch(rCtx, branch, newBranch); err != nil {
				return
			}

			if prog != nil {
				prog.Send(WorkspaceRenameMsg{
					WorkspaceID: wsID,
					NewName:     name,
					NewBranch:   newBranch,
				})
			}
		}()

		return nil
	}
}

// addSessionToWorkspace creates a new Session for the given agent and appends
// it to the workspace's session list. The session starts in waiting state with
// no task — the user must send a prompt to start the agent.
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

// startSessionCmd starts a new agent session in an existing workspace. The
// workspace must already have Branch and WorktreePath set.
func (m *App) startSessionCmd(ws *agent.Workspace, sess *agent.Session) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	worktreePath := ws.WorktreePath
	task := sess.Task

	return func() tea.Msg {
		// fail sends a DoneMsg so the TUI properly transitions. The caller
		// must NOT release the pool — handleDoneMsg does that.
		fail := func(err error) tea.Msg {
			sess.Output.Write("Error: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: err}
		}

		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		if err := m.pool.Acquire(acquireCtx); err != nil {
			// No slot acquired — set status directly and return nil.
			sess.Output.Write("Error: acquire pool slot: " + err.Error())
			sess.Status = agent.StatusErrored
			sess.Error = err
			return nil
		}

		// Pool slot held from here. Errors return DoneMsg; handleDoneMsg releases.

		// Set up MCP config for this agent.
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

// loadDiffCmd returns a tea.Cmd that loads a diff for the given workspace.
func (m *App) loadDiffCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	baseBranch := m.cfg.General.BaseBranch
	return func() tea.Msg {
		diff, err := m.gitMgr.Diff(context.Background(), baseBranch, branch)
		return DiffReadyMsg{WorkspaceID: wsID, Diff: diff, Err: err}
	}
}

// mergeCmd returns a tea.Cmd that merges a workspace's branch.
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

		// Run pre_merge hook.
		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreMerge, hookEnv, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Remove worktree first.
		if err := m.gitMgr.Remove(ctx, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Merge branch.
		if err := m.gitMgr.Merge(ctx, branch, baseBranch); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Run post_merge hook.
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostMerge, hookEnv, m.repoRoot)

		return MergeCompleteMsg{WorkspaceID: wsID}
	}
}

// discardCmd returns a tea.Cmd that discards a workspace.
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

	// Cancel all sessions in this workspace.
	for _, sess := range ws.Sessions {
		if sess.Cancel != nil {
			sess.Cancel()
		}
	}

	return func() tea.Msg {
		ctx := context.Background()

		// Run pre_discard hook.
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PreDiscard, hookEnv, worktreePath)

		if err := m.gitMgr.Discard(ctx, branch, worktreePath); err != nil {
			return DiscardCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		return DiscardCompleteMsg{WorkspaceID: wsID}
	}
}

// sendFollowUpCmd returns a tea.Cmd that sends a follow-up prompt to the
// active session. Creates a new runner for the session with isResume=true.
func (m *App) sendFollowUpCmd(ws *agent.Workspace, sess *agent.Session, prompt string) tea.Cmd {
	wsID := ws.ID
	sessID := sess.ID
	worktreePath := ws.WorktreePath
	return func() tea.Msg {
		// Use a timeout for pool acquisition so we never block forever.
		acquireCtx, acquireCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer acquireCancel()

		// Acquire pool slot.
		if err := m.pool.Acquire(acquireCtx); err != nil {
			sess.Output.Write("Error: " + err.Error())
			return agent.DoneMsg{WorkspaceID: wsID, SessionID: sessID, ExitCode: 1, Err: fmt.Errorf("acquire pool slot: %w", err)}
		}

		// Create new cancellable context for the runner (not tied to acquire timeout).
		sessCtx, cancel := context.WithCancel(context.Background())
		sess.Cancel = cancel
		sess.UpdatedAt = time.Now()

		// Create a new runner for this turn.
		prog := m.program
		runner := agent.NewRunner(wsID, sess, sess.Agent, worktreePath, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
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

// tickCmd returns a tea.Cmd that sends a TickMsg after 1 second.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// animTickCmd returns a tea.Cmd that sends an AnimTickMsg for spinner animation.
func animTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// handleAnimTick advances the spinner animation and refreshes the viewport
// when the current session is active.
func (m *App) handleAnimTick() (tea.Model, tea.Cmd) {
	m.spinnerFrame++
	m.wsView.spinnerFrame = m.spinnerFrame

	// Refresh viewport if current session is active.
	if m.screen == ScreenMain {
		ws := m.selectedWorkspace()
		if ws != nil {
			sess := ws.ActiveSession()
			if sess != nil && (sess.Status == agent.StatusRunning || sess.Status == agent.StatusInitializing) {
				m.wsView.UpdateOutput(ws)
			}
		}
	}

	// Continue ticking only if there are active sessions.
	if m.hasActiveSession() {
		return m, animTickCmd()
	}
	m.animating = false
	return m, nil
}

// hasActiveSession returns true if any session is currently running or initializing.
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

// ensureAnimating starts the animation tick if it's not already running.
func (m *App) ensureAnimating() tea.Cmd {
	if m.animating {
		return nil
	}
	m.animating = true
	return animTickCmd()
}

// --- Helpers ---

// findWorkspace returns the workspace with the given ID, or nil.
func (m *App) findWorkspace(id string) *agent.Workspace {
	for _, ws := range m.workspaces {
		if ws.ID == id {
			return ws
		}
	}
	return nil
}

// removeWorkspace removes the workspace with the given ID from the list.
func (m *App) removeWorkspace(id string) {
	for i, ws := range m.workspaces {
		if ws.ID == id {
			m.workspaces = append(m.workspaces[:i], m.workspaces[i+1:]...)
			// Clamp selection.
			if m.selected >= len(m.workspaces) && len(m.workspaces) > 0 {
				m.selected = len(m.workspaces) - 1
			}
			return
		}
	}
}

// selectedWorkspace returns the workspace currently selected in the sidebar.
func (m *App) selectedWorkspace() *agent.Workspace {
	if len(m.workspaces) == 0 {
		return nil
	}
	idx := m.selected
	if idx < 0 || idx >= len(m.workspaces) {
		return nil
	}
	return m.workspaces[idx]
}

// focusSelected updates the workspace view to show the selected workspace.
func (m *App) focusSelected() {
	ws := m.selectedWorkspace()
	if ws != nil {
		sidebarWidth := 22
		mainWidth := m.width - sidebarWidth
		if mainWidth < 20 {
			mainWidth = 20
		}
		contentHeight := m.height - 2
		if contentHeight < 1 {
			contentHeight = 1
		}
		m.wsView.Focus(ws, mainWidth, contentHeight)
	}
}

// renderHeader builds the header bar.
func (m *App) renderHeader() string {
	title := headerStyle.Render("island")
	repo := footerStyle.Render(" \u2014 " + m.repoName)
	running := fmt.Sprintf("%d/%d agents running", m.pool.RunningCount(), m.cfg.General.MaxConcurrent)
	info := footerStyle.Render(running)

	left := title + repo
	right := info

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + right
}

// renderFooter builds the footer bar with context-sensitive keybinding hints.
func (m *App) renderFooter() string {
	var hints string
	switch m.screen {
	case ScreenMain:
		ws := m.selectedWorkspace()
		if ws != nil && ws.Archived {
			hints = "^n: new  up/down: navigate  ^c: quit"
		} else if ws != nil && ws.PRNumber > 0 {
			hints = "^n: new  ^a: agent  tab: switch  ^d: diff  ^m: merge PR  ^x: discard  ^c: quit"
		} else {
			hints = "^n: new  ^a: agent  tab: switch  ^d: diff  ^p: create PR  ^m: merge  ^x: discard  ^c: quit"
		}
	case ScreenDiff:
		hints = "esc: back  m: merge  x: discard"
	}
	return footerStyle.Render(hints)
}

// handleWorkspaceRename updates the workspace display name and branch after
// Haiku generates a descriptive name.
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

// --- PR Workflow ---

// createPRCmd pushes the branch and creates a GitHub PR.
func (m *App) createPRCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	baseBranch := m.cfg.General.BaseBranch
	wsName := ws.Name

	return func() tea.Msg {
		ctx := context.Background()

		// Push branch to remote.
		if err := m.gitMgr.Push(ctx, branch); err != nil {
			return PRCreateCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Generate title and body from commit log.
		title, body, err := m.gitMgr.GeneratePRDescription(ctx, baseBranch, branch)
		if err != nil || title == "" {
			title = wsName
		}

		// Create PR.
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

// mergePRCmd merges a PR via GitHub, cleans up, and archives.
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

		// Run pre_merge hook.
		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreMerge, hookEnv, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Merge PR on GitHub.
		if err := m.gitMgr.MergePR(ctx, prNumber); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Remove local worktree (branch already deleted by --delete-branch).
		if worktreePath != "" {
			_ = m.gitMgr.Remove(ctx, worktreePath)
		}

		// Pull latest on base branch.
		_ = m.gitMgr.PullBase(ctx, baseBranch)

		// Run post_merge hook.
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostMerge, hookEnv, m.repoRoot)

		return MergeCompleteMsg{WorkspaceID: wsID}
	}
}

// handlePRCreateComplete handles the result of creating a PR.
func (m *App) handlePRCreateComplete(msg PRCreateCompleteMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	if msg.Err != nil {
		sess := ws.ActiveSession()
		if sess != nil {
			sess.Output.Write(erroredStyle.Render("PR creation failed: " + msg.Err.Error()))
			m.wsView.UpdateOutput(ws)
		}
		return m, nil
	}

	ws.PRNumber = msg.PRNumber
	ws.PRURL = msg.PRURL

	sess := ws.ActiveSession()
	if sess != nil {
		sess.Output.Write(completedStyle.Render(fmt.Sprintf("\u2713 PR #%d created: %s", msg.PRNumber, msg.PRURL)))
		m.wsView.UpdateOutput(ws)
	}

	m.saveState()
	return m, nil
}

// --- State Persistence ---

// saveState persists the current state and chat history to disk.
func (m *App) saveState() {
	s := m.buildState()
	if err := state.Save(m.statePath, s); err != nil {
		return // silently ignore save errors
	}

	// Save chat history for each session.
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

// buildState serializes the current app state into a persistable format.
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
			TemplateName:     ws.TemplateName,
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

// restoreState rebuilds workspaces from persisted state.
func (m *App) restoreState(s *state.IslandState) {
	for _, wss := range s.Workspaces {
		// Verify worktree still exists for non-archived workspaces.
		if wss.WorktreePath != "" && !wss.Archived {
			if _, err := os.Stat(wss.WorktreePath); os.IsNotExist(err) {
				continue // worktree gone, skip
			}
		}

		ws := &agent.Workspace{
			ID:           wss.ID,
			Name:         wss.Name,
			Branch:       wss.Branch,
			WorktreePath: wss.WorktreePath,
			TemplateName: wss.TemplateName,
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
				continue // skip sessions with unknown agent type
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

			// Restore chat history into ring buffer.
			lines, err := state.LoadHistory(m.historyDir, wss.ID, ss.ID)
			if err == nil && len(lines) > 0 {
				for _, line := range lines {
					sess.Output.Write(line)
				}
			}

			// Interrupted sessions (were running when island closed) become waiting.
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

// rebuildBranchName replaces the slug portion of a branch name.
// Given "island/<timestamp>-<old>" and a new slug, returns "island/<timestamp>-<new>".
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
