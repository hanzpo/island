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

// WorkspaceName holds the generated display name and branch slug.
type WorkspaceName struct {
	Display string // human-readable, e.g. "Fix auth redirect"
	Slug    string // branch-safe, e.g. "fix-auth-redirect"
}

// GenerateWorkspaceName shells out to `claude` to produce a short, descriptive
// name from a task description. Uses Haiku for speed and cost.
// Returns both a human-readable display name and a branch-safe slug.
func GenerateWorkspaceName(ctx context.Context, task string) (WorkspaceName, error) {
	systemPrompt := `You are a naming function. You receive a coding task and output a 2-4 word title for it. Title case. No punctuation. No explanations. No conversation. Output ONLY the title.

Examples:
- "fix the login bug" -> "Fix Login Bug"
- "add search to users page" -> "Add User Search"
- "refactor database pooling" -> "Refactor DB Pool"
- "hello" -> "General Chat"
- "update CI" -> "Update CI Pipeline"`

	cmd := exec.CommandContext(ctx, "claude",
		"-p", task,
		"--system-prompt", systemPrompt,
		"--model", "claude-haiku-4-5-20251001",
		"--max-turns", "1",
		"--bare",
	)

	out, err := cmd.Output()
	if err != nil {
		return WorkspaceName{}, fmt.Errorf("claude naming call: %w", err)
	}

	display := strings.TrimSpace(string(out))
	// Strip any quotes or trailing punctuation the model may have added.
	display = strings.Trim(display, "\"'`.,!:;")
	display = strings.TrimSpace(display)

	// Take only the first line if the model got chatty.
	if i := strings.IndexByte(display, '\n'); i > 0 {
		display = display[:i]
	}

	// Hard cap at 5 words.
	words := strings.Fields(display)
	if len(words) > 5 {
		words = words[:5]
	}
	display = strings.Join(words, " ")

	if display == "" {
		return WorkspaceName{}, fmt.Errorf("empty name generated")
	}

	// Cap display length.
	if len(display) > 30 {
		display = display[:30]
		if i := strings.LastIndex(display, " "); i > 10 {
			display = display[:i]
		}
	}

	slug := SanitizeBranchName(display)
	if slug == "" {
		return WorkspaceName{}, fmt.Errorf("empty after sanitization")
	}

	return WorkspaceName{Display: display, Slug: slug}, nil
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
