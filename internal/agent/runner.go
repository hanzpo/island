package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

// WorkspaceStatus represents the current state of a workspace or session.
type WorkspaceStatus int

const (
	StatusInitializing WorkspaceStatus = iota
	StatusRunning
	StatusWaiting
	StatusCompleted
	StatusErrored
	StatusCancelled
	StatusMerging
	StatusInReview
	StatusArchived
)

// String returns a human-readable representation of the workspace status.
func (s WorkspaceStatus) String() string {
	switch s {
	case StatusInitializing:
		return "initializing"
	case StatusRunning:
		return "running"
	case StatusWaiting:
		return "waiting"
	case StatusCompleted:
		return "completed"
	case StatusErrored:
		return "errored"
	case StatusCancelled:
		return "cancelled"
	case StatusMerging:
		return "merging"
	case StatusInReview:
		return "in_review"
	case StatusArchived:
		return "archived"
	default:
		return "unknown"
	}
}

// ParseStatus converts a string status back to a WorkspaceStatus.
func ParseStatus(s string) WorkspaceStatus {
	switch s {
	case "initializing":
		return StatusInitializing
	case "running":
		return StatusRunning
	case "waiting":
		return StatusWaiting
	case "completed":
		return StatusCompleted
	case "errored":
		return StatusErrored
	case "cancelled":
		return StatusCancelled
	case "merging":
		return StatusMerging
	case "in_review":
		return StatusInReview
	case "archived":
		return StatusArchived
	default:
		return StatusWaiting
	}
}

// Session represents one agent running in a workspace.
type Session struct {
	ID        string
	Agent     *AgentDef
	Task      string
	Status    WorkspaceStatus
	Output    *RingBuffer
	Stderr    *RingBuffer
	TurnCount int
	ExitCode  int
	Error     error
	Cancel    context.CancelFunc
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Workspace represents an isolated workspace with one branch/worktree
// and potentially multiple agent sessions.
type Workspace struct {
	ID           string
	Name         string
	Branch       string
	WorktreePath string
	TemplateName string
	Sessions     []*Session
	ActiveIdx    int
	PRNumber  int
	PRURL     string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt    time.Time
}

// ActiveSession returns the currently focused session, or nil.
func (w *Workspace) ActiveSession() *Session {
	if w.ActiveIdx < 0 || w.ActiveIdx >= len(w.Sessions) {
		return nil
	}
	return w.Sessions[w.ActiveIdx]
}

// Status returns the "most active" status across all sessions.
func (w *Workspace) Status() WorkspaceStatus {
	if w.Archived {
		return StatusArchived
	}

	if len(w.Sessions) == 0 {
		if w.PRNumber > 0 {
			return StatusInReview
		}
		return StatusInitializing
	}

	hasInitializing := false
	hasWaiting := false
	hasCompleted := false
	hasErrored := false
	allCancelled := true

	for _, s := range w.Sessions {
		switch s.Status {
		case StatusRunning:
			return StatusRunning
		case StatusMerging:
			return StatusMerging
		case StatusInitializing:
			hasInitializing = true
			allCancelled = false
		case StatusWaiting:
			hasWaiting = true
			allCancelled = false
		case StatusCompleted:
			hasCompleted = true
			allCancelled = false
		case StatusErrored:
			hasErrored = true
			allCancelled = false
		case StatusCancelled:
			// stays true
		default:
			allCancelled = false
		}
	}

	if hasInitializing {
		return StatusInitializing
	}
	if w.PRNumber > 0 {
		return StatusInReview
	}
	if hasWaiting {
		return StatusWaiting
	}
	if hasCompleted {
		return StatusCompleted
	}
	if hasErrored {
		return StatusErrored
	}
	if allCancelled {
		return StatusCancelled
	}

	return StatusCompleted
}

// OutputMsg is sent when an agent produces output.
type OutputMsg struct {
	WorkspaceID string
	SessionID   string
	Chunk       string
	IsStderr    bool
}

// DoneMsg is sent when an agent process exits.
type DoneMsg struct {
	WorkspaceID string
	SessionID   string
	ExitCode    int
	Err         error
}

// Runner manages the lifecycle of a single agent session's process.
type Runner struct {
	workspaceID string
	session     *Session
	agent       *AgentDef
	workDir     string
	send        func(interface{})
	cmd         *exec.Cmd
	ptmx        *os.File // PTY master fd, nil if not using PTY
	PtyRows     uint16   // initial PTY rows (set before Start)
	PtyCols     uint16   // initial PTY cols (set before Start)
}

// NewRunner creates a new Runner for the given session and agent.
func NewRunner(workspaceID string, session *Session, agent *AgentDef, workDir string, send func(interface{})) *Runner {
	return &Runner{
		workspaceID: workspaceID,
		session:     session,
		agent:       agent,
		workDir:     workDir,
		send:        send,
		PtyRows:     40,
		PtyCols:     120,
	}
}

// Start spawns the agent process and begins streaming output.
// Uses PTY by default for proper terminal output. Falls back to pipes
// if PTY allocation fails or if the agent uses stream-json format.
func (r *Runner) Start(ctx context.Context, prompt string, isResume bool) error {
	args := r.agent.BuildArgs(prompt, isResume)
	r.cmd = exec.CommandContext(ctx, r.agent.Command, args...)
	r.cmd.Dir = r.workDir
	r.cmd.Env = r.agent.BuildEnv()

	// Use stream-json parsing for agents that explicitly request it.
	if r.agent.OutputFormat == "stream-json" {
		return r.startWithPipes(ctx)
	}

	// Try PTY first; fall back to pipes.
	if err := r.startWithPTY(ctx); err != nil {
		return r.startWithPipes(ctx)
	}
	return nil
}

// startWithPTY allocates a PTY and starts the process.
func (r *Runner) startWithPTY(ctx context.Context) error {
	rows := r.PtyRows
	cols := r.PtyCols
	if rows == 0 {
		rows = 40
	}
	if cols == 0 {
		cols = 120
	}
	ptmx, err := pty.StartWithSize(r.cmd, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
	if err != nil {
		return fmt.Errorf("allocating PTY: %w", err)
	}
	r.ptmx = ptmx

	// Stream PTY output.
	go r.streamPTY(ptmx)

	// Wait for process exit.
	go func() {
		waitErr := r.cmd.Wait()
		// Close PTY after process exits.
		ptmx.Close()

		exitCode := 0
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		r.send(DoneMsg{
			WorkspaceID: r.workspaceID,
			SessionID:   r.session.ID,
			ExitCode:    exitCode,
			Err:         waitErr,
		})
	}()

	return nil
}

// ResizePTY updates the PTY dimensions (called when panel resizes).
func (r *Runner) ResizePTY(rows, cols uint16) {
	if r.ptmx != nil {
		_ = pty.Setsize(r.ptmx, &pty.Winsize{Rows: rows, Cols: cols})
	}
}

// streamPTY reads from the PTY master and processes output.
func (r *Runner) streamPTY(ptmx *os.File) {
	buf := make([]byte, 4096)
	ringBuffer := r.session.Output
	var partial string

	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])

			// Filter out cursor movement and screen clear sequences,
			// but keep color codes.
			chunk = filterANSI(chunk)

			r.send(OutputMsg{
				WorkspaceID: r.workspaceID,
				SessionID:   r.session.ID,
				Chunk:       chunk,
				IsStderr:    false,
			})

			partial += chunk
			for {
				idx := strings.Index(partial, "\n")
				if idx == -1 {
					break
				}
				line := partial[:idx]
				// Strip carriage returns from PTY output.
				line = strings.TrimRight(line, "\r")
				if line != "" {
					ringBuffer.Write(line)
				}
				partial = partial[idx+1:]
			}
		}

		if err != nil {
			break
		}
	}

	if partial != "" {
		partial = strings.TrimRight(partial, "\r")
		if partial != "" {
			ringBuffer.Write(partial)
		}
	}
}

// filterANSI strips cursor movement, screen clear, and other non-color
// escape sequences while keeping SGR (color) codes intact.
func filterANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// CSI sequence: ESC [ <params> <final>
			j := i + 2
			// Read parameter bytes (0x30-0x3f)
			for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
				j++
			}
			// Read intermediate bytes (0x20-0x2f)
			for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
				j++
			}
			// Final byte
			if j < len(s) {
				final := s[j]
				j++
				if final == 'm' {
					// SGR (color) — keep it.
					b.WriteString(s[i:j])
				}
				// All other CSI sequences (cursor movement, clear, etc.) — strip.
				i = j
			} else {
				// Incomplete sequence — skip.
				i = j
			}
		} else if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == ']' {
			// OSC sequence: ESC ] ... BEL or ST — skip entirely.
			j := i + 2
			for j < len(s) && s[j] != '\x07' {
				if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
					j += 2
					break
				}
				j++
			}
			if j < len(s) && s[j] == '\x07' {
				j++
			}
			i = j
		} else if s[i] == '\r' {
			// Skip bare carriage returns.
			i++
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

// startWithPipes uses traditional stdout/stderr pipes (for stream-json or fallback).
func (r *Runner) startWithPipes(ctx context.Context) error {
	// Re-create the command since the previous one may have been consumed by PTY attempt.
	args := r.agent.BuildArgs(r.session.Task, false)
	r.cmd = exec.CommandContext(ctx, r.agent.Command, args...)
	r.cmd.Dir = r.workDir
	r.cmd.Env = r.agent.BuildEnv()

	stdout, err := r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	stderr, err := r.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("starting agent process: %w", err)
	}

	if r.agent.OutputFormat == "stream-json" {
		go r.streamOutputJSON(stdout)
	} else {
		go r.streamOutput(stdout, false)
	}

	go r.streamOutput(stderr, true)

	go func() {
		waitErr := r.cmd.Wait()
		exitCode := 0
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		r.send(DoneMsg{
			WorkspaceID: r.workspaceID,
			SessionID:   r.session.ID,
			ExitCode:    exitCode,
			Err:         waitErr,
		})
	}()

	return nil
}

// streamOutput reads from the given reader and sends chunks to the TUI.
func (r *Runner) streamOutput(reader io.Reader, isStderr bool) {
	buf := make([]byte, 4096)
	var partial string

	ringBuffer := r.session.Output
	if isStderr {
		ringBuffer = r.session.Stderr
	}

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])

			r.send(OutputMsg{
				WorkspaceID: r.workspaceID,
				SessionID:   r.session.ID,
				Chunk:       chunk,
				IsStderr:    isStderr,
			})

			partial += chunk
			for {
				idx := strings.Index(partial, "\n")
				if idx == -1 {
					break
				}
				line := partial[:idx]
				ringBuffer.Write(line)
				partial = partial[idx+1:]
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}

	if partial != "" {
		ringBuffer.Write(partial)
	}
}

// streamOutputJSON reads newline-delimited JSON (stream-json format).
func (r *Runner) streamOutputJSON(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	parser := NewStreamParser()
	ringBuffer := r.session.Output

	for scanner.Scan() {
		line := scanner.Text()
		displayLines := parser.ParseLine(line)

		if len(displayLines) > 0 {
			for _, dl := range displayLines {
				ringBuffer.Write(dl)
			}

			r.send(OutputMsg{
				WorkspaceID: r.workspaceID,
				SessionID:   r.session.ID,
				Chunk:       strings.Join(displayLines, "\n"),
				IsStderr:    false,
			})
		}
	}
}
