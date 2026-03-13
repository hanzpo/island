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
	runners map[string]*Runner // session ID -> runner
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

// Register adds a runner to the pool's tracking map, keyed by session ID.
func (p *Pool) Register(sessionID string, r *Runner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runners[sessionID] = r
}

// Unregister removes a runner from tracking by session ID.
func (p *Pool) Unregister(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.runners, sessionID)
}

// Get returns the runner for a session, or nil if not found.
func (p *Pool) Get(sessionID string) *Runner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runners[sessionID]
}

// RunningCount returns the number of active runners.
func (p *Pool) RunningCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.runners)
}

// CancelAll cancels all running agent sessions for graceful shutdown.
func (p *Pool) CancelAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.runners {
		if r.session != nil && r.session.Cancel != nil {
			r.session.Cancel()
		}
	}
}
