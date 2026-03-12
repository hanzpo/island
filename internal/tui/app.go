package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/agent"
	"github.com/hanz/island/internal/config"
	"github.com/hanz/island/internal/git"
	"github.com/hanz/island/internal/hooks"
)

// Screen represents which top-level screen is currently displayed.
type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenWorkspace
	ScreenDiff
)

// Custom message types for async operations.

// WorkspaceCreatedMsg is sent when a new workspace has been created and its
// agent has been started.
type WorkspaceCreatedMsg struct {
	Workspace *agent.Workspace
}

// WorkspaceCreateErrorMsg is sent when workspace creation fails.
type WorkspaceCreateErrorMsg struct {
	Err error
}

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

// App is the root Bubble Tea model that owns all state and routes to
// sub-views.
type App struct {
	// Dependencies
	cfg        *config.Config
	gitMgr     *git.Manager
	hookRunner *hooks.Runner
	pool       *agent.Pool
	program    *tea.Program
	repoRoot   string
	repoName   string

	// State
	screen     Screen
	workspaces []*agent.Workspace

	// Sub-models
	dashboard DashboardModel
	dialog    DialogModel
	wsView    WorkspaceModel
	diffView  DiffViewModel

	// Window
	width  int
	height int

	// Quit
	confirmQuit bool
}

// NewApp creates and initializes the root TUI model.
func NewApp(cfg *config.Config, repoRoot string) *App {
	gitMgr := git.NewManager(repoRoot, cfg.General.WorktreeDir)
	hookRunner := hooks.NewRunner(repoRoot)
	pool := agent.NewPool(cfg.General.MaxConcurrent)

	return &App{
		cfg:        cfg,
		gitMgr:     gitMgr,
		hookRunner: hookRunner,
		pool:       pool,
		repoRoot:   repoRoot,
		repoName:   filepath.Base(repoRoot),
		screen:     ScreenDashboard,
		dashboard:  newDashboardModel(),
		dialog:     newDialogModel(cfg),
		wsView:     newWorkspaceModel(),
		diffView:   newDiffViewModel(),
	}
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

	case agent.OutputMsg:
		return m.handleOutputMsg(msg)

	case agent.DoneMsg:
		return m.handleDoneMsg(msg)

	case WorkspaceCreatedMsg:
		return m.handleWorkspaceCreated(msg)

	case WorkspaceCreateErrorMsg:
		// TODO: show error in a notification; for now just ignore.
		return m, nil

	case DiffReadyMsg:
		return m.handleDiffReady(msg)

	case MergeCompleteMsg:
		return m.handleMergeComplete(msg)

	case DiscardCompleteMsg:
		return m.handleDiscardComplete(msg)

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Propagate to the active screen's sub-model.
	return m.propagateToScreen(msg)
}

// View implements tea.Model.
func (m *App) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Overlay: dialog takes over the entire screen when open.
	if m.dialog.IsOpen() {
		return m.dialog.View(m.width, m.height)
	}

	// Overlay: quit confirmation takes over the entire screen.
	if m.confirmQuit {
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			dialogStyle.Render("Agents are still running. Quit anyway? (y/n)"),
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

	var content string
	switch m.screen {
	case ScreenDashboard:
		content = m.dashboard.View(m.workspaces, m.width, contentHeight, m.cfg.UI.MinPanelWidth)
	case ScreenWorkspace:
		ws := m.findWorkspace(m.wsView.workspaceID)
		content = m.wsView.View(ws, m.width, contentHeight)
	case ScreenDiff:
		content = m.diffView.View(m.width, contentHeight)
	}

	b.WriteString(content)
	b.WriteByte('\n')

	// Footer.
	footer := m.renderFooter()
	b.WriteString(footer)

	return b.String()
}

// --- Key handling ---

func (m *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Quit confirmation takes priority.
	if m.confirmQuit {
		switch msg.String() {
		case "y", "Y":
			m.pool.CancelAll()
			return m, tea.Quit
		default:
			m.confirmQuit = false
			return m, nil
		}
	}

	// Dialog takes priority when open.
	if m.dialog.IsOpen() {
		cmd := m.dialog.Update(msg)
		// Check if dialog just closed with a confirmed result.
		if !m.dialog.IsOpen() && m.dialog.confirmed {
			createCmd := m.createWorkspaceCmd(
				m.dialog.backendName,
				m.dialog.taskText,
				m.dialog.templateName,
			)
			m.dialog.confirmed = false
			return m, tea.Batch(cmd, createCmd)
		}
		return m, cmd
	}

	// Route to current screen.
	switch m.screen {
	case ScreenDashboard:
		return m.handleDashboardKey(msg)
	case ScreenWorkspace:
		return m.handleWorkspaceKey(msg)
	case ScreenDiff:
		return m.handleDiffKey(msg)
	}

	return m, nil
}

func (m *App) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.dashboard.keys.Quit):
		if m.pool.RunningCount() > 0 {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit

	case key.Matches(msg, m.dashboard.keys.New):
		m.dialog.Open()
		return m, nil

	case key.Matches(msg, m.dashboard.keys.Enter):
		ws := m.selectedWorkspace()
		if ws != nil {
			m.screen = ScreenWorkspace
			m.wsView.Focus(ws, m.width, m.height-2)
		}
		return m, nil

	case key.Matches(msg, m.dashboard.keys.Diff):
		ws := m.selectedWorkspace()
		if ws != nil {
			return m, m.loadDiffCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.dashboard.keys.Merge):
		ws := m.selectedWorkspace()
		if ws != nil && (ws.Status == agent.StatusCompleted || ws.Status == agent.StatusWaiting) {
			return m, m.mergeCmd(ws)
		}
		return m, nil

	case key.Matches(msg, m.dashboard.keys.Discard):
		ws := m.selectedWorkspace()
		if ws != nil {
			return m, m.discardCmd(ws)
		}
		return m, nil

	default:
		cmd := m.dashboard.Update(msg, len(m.workspaces))
		return m, cmd
	}
}

func (m *App) handleWorkspaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(m.wsView.workspaceID)

	switch {
	case key.Matches(msg, m.wsView.keys.Back):
		m.screen = ScreenDashboard
		m.wsView.input.Blur()
		return m, nil

	case key.Matches(msg, m.wsView.keys.Diff):
		if ws != nil && !m.wsView.input.Focused() {
			return m, m.loadDiffCmd(ws)
		}

	case key.Matches(msg, m.wsView.keys.Cancel):
		if ws != nil && !m.wsView.input.Focused() && ws.Cancel != nil {
			ws.Cancel()
			return m, nil
		}
	}

	// Handle Enter: send follow-up.
	if msg.String() == "enter" && m.wsView.input.Focused() {
		value := strings.TrimSpace(m.wsView.input.Value())
		if value != "" && ws != nil {
			m.wsView.input.SetValue("")
			return m, m.sendFollowUpCmd(ws, value)
		}
		return m, nil
	}

	cmd := m.wsView.Update(msg, ws)
	return m, cmd
}

func (m *App) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.diffView.keys.Back):
		// Return to whichever screen we came from.
		if m.wsView.workspaceID != "" && m.wsView.workspaceID == m.diffView.workspaceID {
			m.screen = ScreenWorkspace
		} else {
			m.screen = ScreenDashboard
		}
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
			return m, m.discardCmd(ws)
		}
		return m, nil
	}

	cmd := m.diffView.Update(msg)
	return m, cmd
}

// --- Async message handlers ---

func (m *App) handleOutputMsg(msg agent.OutputMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	if msg.IsStderr {
		if ws.Stderr != nil {
			ws.Stderr.Write(msg.Chunk)
		}
	} else {
		if ws.Output != nil {
			ws.Output.Write(msg.Chunk)
		}
	}
	ws.UpdatedAt = time.Now()

	// If we are viewing this workspace, update the viewport.
	if m.screen == ScreenWorkspace && m.wsView.workspaceID == ws.ID {
		m.wsView.UpdateOutput(ws)
	}

	return m, nil
}

func (m *App) handleDoneMsg(msg agent.DoneMsg) (tea.Model, tea.Cmd) {
	ws := m.findWorkspace(msg.WorkspaceID)
	if ws == nil {
		return m, nil
	}

	ws.ExitCode = msg.ExitCode
	ws.UpdatedAt = time.Now()
	if msg.Err != nil {
		ws.Status = agent.StatusErrored
		ws.Error = msg.Err
	} else if msg.ExitCode == 0 {
		ws.Status = agent.StatusCompleted
	} else {
		ws.Status = agent.StatusErrored
	}

	m.pool.Release()
	m.pool.Unregister(ws.ID)

	return m, nil
}

func (m *App) handleWorkspaceCreated(msg WorkspaceCreatedMsg) (tea.Model, tea.Cmd) {
	m.workspaces = append(m.workspaces, msg.Workspace)
	// Select the new workspace on the dashboard.
	m.dashboard.selected = len(m.workspaces) - 1
	return m, nil
}

func (m *App) handleDiffReady(msg DiffReadyMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		// TODO: show error; for now just stay on current screen.
		return m, nil
	}
	m.diffView.SetDiff(msg.WorkspaceID, msg.Diff, m.width, m.height-2)
	m.screen = ScreenDiff
	return m, nil
}

func (m *App) handleMergeComplete(msg MergeCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	m.removeWorkspace(msg.WorkspaceID)
	m.screen = ScreenDashboard
	return m, nil
}

func (m *App) handleDiscardComplete(msg DiscardCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		return m, nil
	}
	m.removeWorkspace(msg.WorkspaceID)
	m.screen = ScreenDashboard
	return m, nil
}

// --- Propagate to sub-models ---

func (m *App) propagateToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case ScreenDashboard:
		cmd := m.dashboard.Update(msg, len(m.workspaces))
		return m, cmd
	case ScreenWorkspace:
		ws := m.findWorkspace(m.wsView.workspaceID)
		cmd := m.wsView.Update(msg, ws)
		return m, cmd
	case ScreenDiff:
		cmd := m.diffView.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Commands ---

// createWorkspaceCmd returns a tea.Cmd that creates a workspace, sets up the
// git worktree, runs hooks/init, starts the agent runner, and sends back a
// WorkspaceCreatedMsg (or WorkspaceCreateErrorMsg on failure).
func (m *App) createWorkspaceCmd(backendName, task, templateName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Acquire pool slot.
		if err := m.pool.Acquire(ctx); err != nil {
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("acquire pool slot: %w", err)}
		}

		wsID := fmt.Sprintf("ws-%d", time.Now().UnixNano())

		// Sanitize task for branch name.
		slug := sanitizeSlug(task)

		// Create git worktree.
		branch, worktreePath, err := m.gitMgr.Create(ctx, m.cfg.General.BaseBranch, slug)
		if err != nil {
			m.pool.Release()
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("create worktree: %w", err)}
		}

		// Build hook env.
		hookEnv := hooks.Env{
			WorkspaceID:  wsID,
			Branch:       branch,
			Backend:      backendName,
			WorktreePath: worktreePath,
			Task:         task,
			RepoRoot:     m.repoRoot,
		}

		// Run pre_workspace_create hook.
		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreWorkspaceCreate, hookEnv, worktreePath); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			m.pool.Release()
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("pre_workspace_create hook: %w", err)}
		}

		// Look up backend config and create Backend.
		bcfg, ok := m.cfg.Backends[backendName]
		if !ok {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			m.pool.Release()
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("backend %q not found in config", backendName)}
		}
		backend := agent.BackendFromConfig(backendName, bcfg)

		// Set up MCP config.
		if err := backend.SetupMCPConfig(worktreePath, m.cfg.MCP); err != nil {
			_ = m.gitMgr.Remove(ctx, worktreePath)
			m.pool.Release()
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("setup MCP config: %w", err)}
		}

		// Run init script.
		if m.cfg.Init.Script != "" {
			if _, err := m.hookRunner.Run(ctx, m.cfg.Init.Script, hookEnv, worktreePath); err != nil {
				_ = m.gitMgr.Remove(ctx, worktreePath)
				m.pool.Release()
				return WorkspaceCreateErrorMsg{Err: fmt.Errorf("init script: %w", err)}
			}
		}

		// Resolve prompt.
		prompt := task
		if templateName != "" {
			if tmpl, ok := m.cfg.Templates[templateName]; ok {
				prompt = config.ApplyTemplate(tmpl, task)
			}
		}

		// Build Workspace.
		wsCtx, cancel := context.WithCancel(ctx)
		bufSize := m.cfg.General.OutputBufferSize
		ws := &agent.Workspace{
			ID:           wsID,
			Task:         task,
			Backend:      backend,
			Status:       agent.StatusInitializing,
			Branch:       branch,
			WorktreePath: worktreePath,
			Output:       agent.NewRingBuffer(bufSize),
			Stderr:       agent.NewRingBuffer(bufSize),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Cancel:       cancel,
		}

		// Create runner; send callback pushes messages via the tea.Program.
		prog := m.program
		runner := agent.NewRunner(ws, backend, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		m.pool.Register(wsID, runner)

		// Start the agent.
		ws.Status = agent.StatusRunning
		if err := runner.Start(wsCtx, prompt, false); err != nil {
			cancel()
			_ = m.gitMgr.Remove(ctx, worktreePath)
			m.pool.Release()
			m.pool.Unregister(wsID)
			return WorkspaceCreateErrorMsg{Err: fmt.Errorf("start agent: %w", err)}
		}

		// Run post_workspace_create hook (non-blocking error).
		_, _ = m.hookRunner.Run(ctx, m.cfg.Hooks.PostWorkspaceCreate, hookEnv, worktreePath)

		return WorkspaceCreatedMsg{Workspace: ws}
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

	hookEnv := hooks.Env{
		WorkspaceID:  wsID,
		Branch:       branch,
		Backend:      "",
		WorktreePath: worktreePath,
		RepoRoot:     m.repoRoot,
	}
	if ws.Backend != nil {
		hookEnv.Backend = ws.Backend.Name
	}

	return func() tea.Msg {
		ctx := context.Background()

		// Run pre_merge hook.
		if _, err := m.hookRunner.Run(ctx, m.cfg.Hooks.PreMerge, hookEnv, worktreePath); err != nil {
			return MergeCompleteMsg{WorkspaceID: wsID, Err: err}
		}

		// Remove worktree first (merge needs to checkout baseBranch).
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

// discardCmd returns a tea.Cmd that discards a workspace (removes worktree
// and deletes the branch).
func (m *App) discardCmd(ws *agent.Workspace) tea.Cmd {
	wsID := ws.ID
	branch := ws.Branch
	worktreePath := ws.WorktreePath

	hookEnv := hooks.Env{
		WorkspaceID:  wsID,
		Branch:       branch,
		WorktreePath: worktreePath,
		RepoRoot:     m.repoRoot,
	}
	if ws.Backend != nil {
		hookEnv.Backend = ws.Backend.Name
	}

	// Cancel the agent if still running.
	if ws.Cancel != nil {
		ws.Cancel()
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

// sendFollowUpCmd returns a tea.Cmd that sends a follow-up prompt to a
// workspace's agent runner. Creates a new runner for the follow-up turn since
// the previous runner was unregistered on completion.
func (m *App) sendFollowUpCmd(ws *agent.Workspace, prompt string) tea.Cmd {
	wsID := ws.ID
	return func() tea.Msg {
		ctx := context.Background()

		// Acquire pool slot (was released when previous turn completed).
		if err := m.pool.Acquire(ctx); err != nil {
			return agent.DoneMsg{WorkspaceID: wsID, ExitCode: 1, Err: fmt.Errorf("acquire pool slot: %w", err)}
		}

		// Write turn separator to output buffer.
		ws.TurnCount++
		sep := fmt.Sprintf("─── Turn %d ───────────────────────────────────────────────────────────────────\n> User: %s", ws.TurnCount, prompt)
		ws.Output.Write(sep)

		// Create new cancellable context.
		wsCtx, cancel := context.WithCancel(ctx)
		ws.Cancel = cancel
		ws.Status = agent.StatusRunning
		ws.UpdatedAt = time.Now()

		// Create a new runner for this turn.
		prog := m.program
		runner := agent.NewRunner(ws, ws.Backend, func(msg interface{}) {
			if prog != nil {
				prog.Send(msg)
			}
		})
		m.pool.Register(wsID, runner)

		if err := runner.Start(wsCtx, prompt, true); err != nil {
			cancel()
			m.pool.Release()
			m.pool.Unregister(wsID)
			return agent.DoneMsg{WorkspaceID: wsID, ExitCode: 1, Err: fmt.Errorf("start follow-up: %w", err)}
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
			// Clamp dashboard selection.
			if m.dashboard.selected >= len(m.workspaces) && len(m.workspaces) > 0 {
				m.dashboard.selected = len(m.workspaces) - 1
			}
			return
		}
	}
}

// selectedWorkspace returns the workspace currently selected on the dashboard.
func (m *App) selectedWorkspace() *agent.Workspace {
	if len(m.workspaces) == 0 {
		return nil
	}
	idx := m.dashboard.selected
	if idx < 0 || idx >= len(m.workspaces) {
		return nil
	}
	return m.workspaces[idx]
}

// renderHeader builds the header bar.
func (m *App) renderHeader() string {
	title := headerStyle.Render("island")
	repo := footerStyle.Render(" " + m.repoName)
	running := fmt.Sprintf(" %d/%d agents", m.pool.RunningCount(), m.cfg.General.MaxConcurrent)
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
	case ScreenDashboard:
		hints = "n: new  enter: focus  d: diff  m: merge  x: discard  q: quit"
	case ScreenWorkspace:
		hints = "esc: back  d: diff  c: cancel  pgup/pgdn: scroll"
	case ScreenDiff:
		hints = "m: merge  x: discard  esc: back  pgup/pgdn: scroll"
	}
	return footerStyle.Render(hints)
}

// sanitizeSlug converts a task description into a branch-name-safe slug.
func sanitizeSlug(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = s[:50]
		s = strings.TrimRight(s, "-")
	}
	return s
}
