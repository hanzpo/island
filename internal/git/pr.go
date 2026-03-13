package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Push pushes a branch to the remote origin with tracking.
func (m *Manager) Push(ctx context.Context, branch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branch)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push: %w: %s", err, stderr.String())
	}

	return nil
}

// PRInfo holds the result of creating a pull request.
type PRInfo struct {
	Number int
	URL    string
}

// CreatePR creates a GitHub pull request using the gh CLI.
func (m *Manager) CreatePR(ctx context.Context, branch, baseBranch, title, body string) (*PRInfo, error) {
	args := []string{"pr", "create",
		"--base", baseBranch,
		"--head", branch,
		"--title", title,
		"--body", body,
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = m.repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr create: %w: %s", err, stderr.String())
	}

	url := strings.TrimSpace(stdout.String())

	// Parse PR number from URL (e.g. https://github.com/user/repo/pull/123).
	number := 0
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		fmt.Sscanf(parts[len(parts)-1], "%d", &number)
	}

	return &PRInfo{Number: number, URL: url}, nil
}

// MergePR merges a pull request via the gh CLI with squash merge.
func (m *Manager) MergePR(ctx context.Context, prNumber int) error {
	cmd := exec.CommandContext(ctx, "gh", "pr", "merge",
		fmt.Sprintf("%d", prNumber),
		"--squash",
		"--delete-branch",
	)
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh pr merge: %w: %s", err, stderr.String())
	}

	return nil
}

// GeneratePRDescription generates a PR title and body from git commit log.
func (m *Manager) GeneratePRDescription(ctx context.Context, baseBranch, branch string) (title string, body string, err error) {
	cmd := exec.CommandContext(ctx, "git", "log", baseBranch+".."+branch, "--pretty=format:%s")
	cmd.Dir = m.repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("git log: %w: %s", err, stderr.String())
	}

	commits := strings.TrimSpace(stdout.String())
	if commits == "" {
		return "", "", nil
	}

	lines := strings.Split(commits, "\n")

	// First commit message as title.
	if len(lines) > 0 {
		title = lines[0]
	}

	// Remaining commits as body.
	if len(lines) > 1 {
		var bodyLines []string
		bodyLines = append(bodyLines, "## Changes\n")
		for _, line := range lines[1:] {
			if line != "" {
				bodyLines = append(bodyLines, "- "+line)
			}
		}
		body = strings.Join(bodyLines, "\n")
	}

	return title, body, nil
}

// PullBase pulls the latest changes on the base branch.
func (m *Manager) PullBase(ctx context.Context, baseBranch string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.runGit(ctx, "checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout %s: %w", baseBranch, err)
	}

	cmd := exec.CommandContext(ctx, "git", "pull", "--ff-only")
	cmd.Dir = m.repoRoot

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Non-fatal: pull might fail if not tracking remote.
	_ = cmd.Run()

	return nil
}
