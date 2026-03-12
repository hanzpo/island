package agent

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// WorkspaceStatus represents the current state of a workspace.
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

// Workspace represents a single agent workspace with its state and output buffers.
type Workspace struct {
	ID           string
	Task         string
	Backend      *Backend
	Status       WorkspaceStatus
	Branch       string
	WorktreePath string
	Output       *RingBuffer
	Stderr       *RingBuffer
	TurnCount    int
	ExitCode     int
	Error        error
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Cancel       context.CancelFunc
}

// OutputMsg is sent when an agent produces output.
type OutputMsg struct {
	WorkspaceID string
	Chunk       string
	IsStderr    bool
}

// DoneMsg is sent when an agent process exits.
type DoneMsg struct {
	WorkspaceID string
	ExitCode    int
	Err         error
}

// Runner manages the lifecycle of an agent process.
type Runner struct {
	workspace *Workspace
	backend   *Backend
	send      func(interface{})
	cmd       *exec.Cmd
}

// NewRunner creates a new Runner for the given workspace and backend.
func NewRunner(ws *Workspace, backend *Backend, send func(interface{})) *Runner {
	return &Runner{
		workspace: ws,
		backend:   backend,
		send:      send,
	}
}

// Start spawns the agent process and begins streaming output. It uses
// os/exec.CommandContext for cancellation support. Output is read using raw
// Read() calls for smooth streaming.
func (r *Runner) Start(ctx context.Context, prompt string, isResume bool) error {
	args := r.backend.BuildArgs(prompt, isResume)

	r.cmd = exec.CommandContext(ctx, r.backend.Command, args...)
	r.cmd.Dir = r.workspace.WorktreePath
	r.cmd.Env = r.backend.BuildEnv()

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

	// Stream stdout.
	go r.streamOutput(stdout, false)

	// Stream stderr.
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
			WorkspaceID: r.workspace.ID,
			ExitCode:    exitCode,
			Err:         waitErr,
		})
	}()

	return nil
}

// streamOutput reads from the given reader using raw Read() calls and sends
// OutputMsg chunks. It also writes complete lines to the appropriate ring buffer.
func (r *Runner) streamOutput(reader io.Reader, isStderr bool) {
	buf := make([]byte, 4096)
	var partial string

	ringBuffer := r.workspace.Output
	if isStderr {
		ringBuffer = r.workspace.Stderr
	}

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])

			// Send raw chunk to TUI for immediate display.
			r.send(OutputMsg{
				WorkspaceID: r.workspace.ID,
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
