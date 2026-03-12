package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Diff returns the diff between baseBranch and workspaceBranch using
// three-dot notation (baseBranch...workspaceBranch). The command is run
// from the repo root.
func (m *Manager) Diff(ctx context.Context, baseBranch, workspaceBranch string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", baseBranch+"..."+workspaceBranch)
	cmd.Dir = m.repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git diff: %w: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
