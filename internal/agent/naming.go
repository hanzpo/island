package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"regexp"
	"strings"
)

// cityNames is a pool of short city names used as temporary workspace
// identifiers before a descriptive name is generated.
var cityNames = []string{
	"tokyo", "paris", "london", "berlin", "oslo",
	"cairo", "lima", "rome", "seoul", "dublin",
	"vienna", "zurich", "prague", "lisbon", "nairobi",
	"havana", "austin", "denver", "portland", "milan",
	"sydney", "mumbai", "lagos", "hanoi", "bogota",
	"athens", "geneva", "lyon", "stockholm", "phoenix",
	"bruges", "porto", "malaga", "kyoto", "cusco",
	"fez", "cork", "bath", "siena", "baku",
}

// PickCityName returns a random city name for use as a temporary workspace name.
func PickCityName() string {
	return cityNames[rand.Intn(len(cityNames))]
}

// GenerateBranchName shells out to `claude` to produce a short, descriptive
// git-branch-safe name from a task description. Uses Haiku via --model for
// speed and cost. Returns an error if claude is not on PATH or the call fails.
func GenerateBranchName(ctx context.Context, task string) (string, error) {
	prompt := "Generate a short (2-4 word) git branch name for this coding task. Output ONLY the branch name: lowercase, hyphens between words, no spaces, no special characters, no explanations. Examples: fix-auth-redirect, add-user-search, refactor-db-pool\n\nTask: " + task

	cmd := exec.CommandContext(ctx, "claude",
		"-p", prompt,
		"--model", "claude-haiku-4-5-20251001",
		"--max-turns", "1",
	)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude naming call: %w", err)
	}

	name := SanitizeBranchName(string(out))
	if name == "" {
		return "", fmt.Errorf("empty after sanitization")
	}
	return name, nil
}

// SanitizeBranchName cleans a string for use in a git branch name.
func SanitizeBranchName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = s[:50]
		s = strings.TrimRight(s, "-")
	}
	return s
}
