package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Merge merges branch into baseBranch. It first checks out baseBranch, then
// attempts a fast-forward merge. If that fails, it falls back to a regular
// merge with --no-edit.
func (m *Manager) Merge(ctx context.Context, branch, baseBranch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Step 1: checkout baseBranch
	if err := m.runGit(ctx, "checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout %s: %w", baseBranch, err)
	}

	// Step 2: try fast-forward merge
	if err := m.runGit(ctx, "merge", "--ff-only", branch); err == nil {
		return nil
	}

	// Step 3: fall back to regular merge
	if err := m.runGit(ctx, "merge", "--no-edit", branch); err != nil {
		return fmt.Errorf("merge %s into %s: %w", branch, baseBranch, err)
	}

	return nil
}

// Discard removes the worktree and deletes the branch.
func (m *Manager) Discard(ctx context.Context, branch, worktreePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Step 1: remove worktree
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w: %s", err, stderr.String())
	}

	// Step 2: delete branch
	if err := m.runGit(ctx, "branch", "-D", branch); err != nil {
		return fmt.Errorf("delete branch %s: %w", branch, err)
	}

	return nil
}

// runGit runs a git command in the repo root directory.
func (m *Manager) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}

	return nil
}
