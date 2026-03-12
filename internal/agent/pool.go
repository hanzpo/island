package agent

import (
	"context"
	"fmt"
	"sync"
)

// Pool is a semaphore-based pool that limits concurrent agent processes.
type Pool struct {
	sem     chan struct{}
	mu      sync.Mutex
	runners map[string]*Runner
}

// NewPool creates a new Pool with the given concurrency limit.
func NewPool(maxConcurrent int) *Pool {
	return &Pool{
		sem:     make(chan struct{}, maxConcurrent),
		runners: make(map[string]*Runner),
	}
}

// Acquire blocks until a slot is available or ctx is cancelled.
func (p *Pool) Acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("acquiring pool slot: %w", ctx.Err())
	}
}

// Release returns a slot to the pool.
func (p *Pool) Release() {
	<-p.sem
}

// Register adds a runner to the pool's tracking map.
func (p *Pool) Register(workspaceID string, r *Runner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runners[workspaceID] = r
}

// Unregister removes a runner from tracking.
func (p *Pool) Unregister(workspaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.runners, workspaceID)
}

// Get returns the runner for a workspace, or nil if not found.
func (p *Pool) Get(workspaceID string) *Runner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runners[workspaceID]
}

// RunningCount returns the number of active runners.
func (p *Pool) RunningCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.runners)
}

// CancelAll cancels all running agents for graceful shutdown.
func (p *Pool) CancelAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.runners {
		if r.workspace != nil && r.workspace.Cancel != nil {
			r.workspace.Cancel()
		}
	}
}
