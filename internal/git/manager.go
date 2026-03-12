package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Manager manages git worktrees. All create/remove operations are serialized
// behind a mutex.
type Manager struct {
	repoRoot    string
	worktreeDir string // absolute path
	mu          sync.Mutex
}

// WorktreeInfo holds information about a git worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
}

// NewManager creates a new worktree Manager. worktreeDir is relative to
// repoRoot and is resolved to an absolute path.
func NewManager(repoRoot, worktreeDir string) *Manager {
	absWorktreeDir := worktreeDir
	if !filepath.IsAbs(worktreeDir) {
		absWorktreeDir = filepath.Join(repoRoot, worktreeDir)
	}
	return &Manager{
		repoRoot:    repoRoot,
		worktreeDir: absWorktreeDir,
	}
}

// sanitizeSlug converts a task description into a valid branch name component.
// Lowercases, replaces non-alphanumeric characters with hyphens, and truncates
// to 50 characters.
func sanitizeSlug(slug string) string {
	slug = strings.ToLower(slug)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug = re.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}
	return slug
}

// Create creates a new git worktree with a branch based on baseBranch.
// Branch naming: island/<unix-timestamp>-<sanitized-taskSlug>.
func (m *Manager) Create(ctx context.Context, baseBranch, taskSlug string) (branch, worktreePath string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sanitized := sanitizeSlug(taskSlug)
	timestamp := time.Now().Unix()
	branch = fmt.Sprintf("island/%d-%s", timestamp, sanitized)
	worktreePath = filepath.Join(m.worktreeDir, branch)

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, worktreePath, baseBranch)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w: %s", err, stderr.String())
	}

	return branch, worktreePath, nil
}

// Remove force-removes a git worktree at the given path.
func (m *Manager) Remove(ctx context.Context, worktreePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w: %s", err, stderr.String())
	}

	return nil
}

// List returns all git worktrees by parsing "git worktree list --porcelain".
func (m *Manager) List(ctx context.Context) ([]WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git worktree list: %w: %s", err, stderr.String())
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo

	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix.
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
			// Empty line separates entries; flush if we have data.
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
			}
		}
	}
	// Flush last entry if not already flushed.
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// Prune prunes stale worktree entries.
func (m *Manager) Prune(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree prune: %w: %s", err, stderr.String())
	}

	return nil
}
