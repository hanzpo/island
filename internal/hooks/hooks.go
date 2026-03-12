package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Runner executes shell commands at lifecycle events.
type Runner struct {
	repoRoot string
}

// Env holds the environment variables passed to hook commands.
type Env struct {
	WorkspaceID  string
	Branch       string
	Backend      string
	WorktreePath string
	Task         string
	RepoRoot     string
}

// NewRunner creates a new hook Runner.
func NewRunner(repoRoot string) *Runner {
	return &Runner{repoRoot: repoRoot}
}

// Run executes a hook command via "sh -c". If hook is empty, returns nil
// immediately. Sets ISLAND_* environment variables from env. workDir is the
// working directory for the command.
func (r *Runner) Run(ctx context.Context, hook string, env Env, workDir string) (string, error) {
	if hook == "" {
		return "", nil
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", hook)
	cmd.Dir = workDir

	// Inherit current environment and add ISLAND_* vars.
	cmd.Env = append(os.Environ(), env.envVars()...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("hook %q failed: %w (output: %s)", hook, err, buf.String())
	}

	return buf.String(), nil
}

// envVars converts Env to a slice of KEY=VALUE strings with ISLAND_ prefix.
func (e Env) envVars() []string {
	var vars []string
	if e.WorkspaceID != "" {
		vars = append(vars, "ISLAND_WORKSPACE_ID="+e.WorkspaceID)
	}
	if e.Branch != "" {
		vars = append(vars, "ISLAND_BRANCH="+e.Branch)
	}
	if e.Backend != "" {
		vars = append(vars, "ISLAND_BACKEND="+e.Backend)
	}
	if e.WorktreePath != "" {
		vars = append(vars, "ISLAND_WORKTREE_PATH="+e.WorktreePath)
	}
	if e.Task != "" {
		vars = append(vars, "ISLAND_TASK="+e.Task)
	}
	if e.RepoRoot != "" {
		vars = append(vars, "ISLAND_REPO_ROOT="+e.RepoRoot)
	}
	return vars
}
