package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureGitignore adds .island/ to .gitignore in repoRoot if not already
// present. Creates .gitignore if it doesn't exist.
func EnsureGitignore(repoRoot string) error {
	gitignorePath := filepath.Join(repoRoot, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}

	entry := ".island/"

	// Check if .island/ is already in the file.
	if len(content) > 0 {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == entry || trimmed == ".island" {
				return nil // Already present.
			}
		}
	}

	// Append .island/ to the file.
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	// If file exists and doesn't end with newline, add one first.
	if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return fmt.Errorf("writing newline to .gitignore: %w", err)
		}
	}

	if _, err := f.WriteString(entry + "\n"); err != nil {
		return fmt.Errorf("writing to .gitignore: %w", err)
	}

	return nil
}
