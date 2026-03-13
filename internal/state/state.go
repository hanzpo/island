package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const stateVersion = 1

// IslandState is the top-level persistent state for an island.
type IslandState struct {
	Version           int              `json:"version"`
	SelectedWorkspace int              `json:"selected_workspace"`
	Workspaces        []WorkspaceState `json:"workspaces"`
}

// WorkspaceState is the persistent state for a single workspace.
type WorkspaceState struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Branch           string         `json:"branch"`
	WorktreePath     string         `json:"worktree_path"`
	TemplateName     string         `json:"template_name,omitempty"`
	PRNumber         int            `json:"pr_number,omitempty"`
	PRURL            string         `json:"pr_url,omitempty"`
	Archived         bool           `json:"archived,omitempty"`
	ActiveSessionIdx int            `json:"active_session_idx"`
	Sessions         []SessionState `json:"sessions"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// SessionState is the persistent state for a single agent session.
type SessionState struct {
	ID        string    `json:"id"`
	AgentName string    `json:"agent_name"`
	Task      string    `json:"task"`
	Status    string    `json:"status"`
	TurnCount int       `json:"turn_count"`
	ExitCode  int       `json:"exit_code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Save writes the island state to the given path as JSON.
// Uses atomic write via a temp file.
func Save(path string, s *IslandState) error {
	s.Version = stateVersion

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// Load reads the island state from the given path.
// Returns (nil, nil) if the file does not exist.
func Load(path string) (*IslandState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var s IslandState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	return &s, nil
}

// SaveHistory writes output lines to a history file for a session.
func SaveHistory(historyDir, workspaceID, sessionID string, lines []string) error {
	dir := filepath.Join(historyDir, workspaceID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating history directory: %w", err)
	}

	path := filepath.Join(dir, sessionID+".log")
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing history: %w", err)
	}

	return nil
}

// LoadHistory reads output lines from a session's history file.
// Returns empty slice if the file doesn't exist.
func LoadHistory(historyDir, workspaceID, sessionID string) ([]string, error) {
	path := filepath.Join(historyDir, workspaceID, sessionID+".log")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading history: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	return strings.Split(string(data), "\n"), nil
}

// RemoveHistory deletes all history files for a workspace.
func RemoveHistory(historyDir, workspaceID string) error {
	dir := filepath.Join(historyDir, workspaceID)
	return os.RemoveAll(dir)
}
