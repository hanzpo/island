package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
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
	default:
		return "unknown"
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
	Name         string // display name derived from task
	Branch       string
	WorktreePath string
	Sessions     []*Session
	ActiveIdx    int // index of focused session in TUI
	CreatedAt    time.Time
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
// Priority: Running > Initializing > Waiting > Completed > Errored > Cancelled
func (w *Workspace) Status() WorkspaceStatus {
	if len(w.Sessions) == 0 {
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
	workDir     string // worktree path
	send        func(interface{})
	cmd         *exec.Cmd
}

// NewRunner creates a new Runner for the given session and agent.
func NewRunner(workspaceID string, session *Session, agent *AgentDef, workDir string, send func(interface{})) *Runner {
	return &Runner{
		workspaceID: workspaceID,
		session:     session,
		agent:       agent,
		workDir:     workDir,
		send:        send,
	}
}

// Start spawns the agent process and begins streaming output. It uses
// os/exec.CommandContext for cancellation support. Output is read using raw
// Read() calls for smooth streaming.
//
// The runner writes ONLY to the session's ring buffers. It sends OutputMsg
// to the TUI for notification only (the TUI should NOT also write to the
// ring buffer — it should just refresh the viewport from the ring buffer).
func (r *Runner) Start(ctx context.Context, prompt string, isResume bool) error {
	args := r.agent.BuildArgs(prompt, isResume)

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

	// Stream stdout — use JSON parser for stream-json agents.
	if r.agent.OutputFormat == "stream-json" {
		go r.streamOutputJSON(stdout)
	} else {
		go r.streamOutput(stdout, false)
	}

	// Stderr is always raw text.
	go r.streamOutput(stderr, true)

	// Wait for process exit.
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

// streamOutput reads from the given reader using raw Read() calls and sends
// OutputMsg chunks to the TUI for notification. It writes complete lines to
// the session's ring buffer. The TUI should NOT also write to the ring buffer;
// it should refresh its viewport from the ring buffer contents.
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

			// Send raw chunk to TUI for notification that new output arrived.
			r.send(OutputMsg{
				WorkspaceID: r.workspaceID,
				SessionID:   r.session.ID,
				Chunk:       chunk,
				IsStderr:    isStderr,
			})

			// Split on newlines for ring buffer storage.
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

	// Flush any remaining partial line.
	if partial != "" {
		ringBuffer.Write(partial)
	}
}

// streamOutputJSON reads newline-delimited JSON from stdout (stream-json format),
// parses each event, and writes formatted display lines to the ring buffer.
// This gives users visibility into tool calls, text output, and results — similar
// to the interactive Claude Code CLI experience.
func (r *Runner) streamOutputJSON(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	// Large buffer for JSON lines that may contain file contents in tool results.
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
