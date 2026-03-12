package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanz/island/internal/config"
)

// Backend represents an AI coding agent backend (e.g., claude, codex).
type Backend struct {
	Name         string
	Command      string
	FirstRunArgs []string
	ResumeArgs   []string
	ExtraArgs    []string
	Env          map[string]string
	Model        string
	Permissions  string
}

// BackendFromConfig creates a Backend from a config.BackendConfig.
func BackendFromConfig(name string, cfg config.BackendConfig) *Backend {
	return &Backend{
		Name:         name,
		Command:      cfg.Command,
		FirstRunArgs: cfg.FirstRunArgs,
		ResumeArgs:   cfg.ResumeArgs,
		ExtraArgs:    cfg.ExtraArgs,
		Env:          cfg.Env,
		Model:        cfg.Model,
		Permissions:  cfg.Permissions,
	}
}

// BuildArgs returns the full argument list for an invocation.
// Replaces {{prompt}} in the appropriate args template.
// If isResume, uses ResumeArgs; otherwise FirstRunArgs.
// Appends ExtraArgs. If Model is set, appends --model <model>.
// If Permissions is set, splits by spaces and prepends.
func (b *Backend) BuildArgs(prompt string, isResume bool) []string {
	var args []string

	// Prepend permissions flags if set.
	if b.Permissions != "" {
		parts := strings.Fields(b.Permissions)
		args = append(args, parts...)
	}

	// Choose the base args template.
	var baseArgs []string
	if isResume {
		baseArgs = b.ResumeArgs
	} else {
		baseArgs = b.FirstRunArgs
	}

	// Replace {{prompt}} placeholder in each arg.
	for _, arg := range baseArgs {
		args = append(args, strings.ReplaceAll(arg, "{{prompt}}", prompt))
	}

	// Append extra args.
	args = append(args, b.ExtraArgs...)

	// Append model flag if set.
	if b.Model != "" {
		args = append(args, "--model", b.Model)
	}

	return args
}

// BuildEnv returns os.Environ() merged with the backend's extra env vars.
// Backend env vars override existing environment variables.
func (b *Backend) BuildEnv() []string {
	env := os.Environ()
	for k, v := range b.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// mcpServerJSON is the JSON representation of an MCP server entry.
type mcpServerJSON struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// SetupMCPConfig writes MCP server config into the worktree for this backend.
// Supported backends: "claude" (.mcp.json), "codex" (.codex/config.json),
// "opencode" (opencode.json). Unknown backends are silently skipped.
func (b *Backend) SetupMCPConfig(worktreePath string, servers map[string]config.MCPServer) error {
	if len(servers) == 0 {
		return nil
	}

	// Build the servers map for JSON.
	serversJSON := make(map[string]mcpServerJSON, len(servers))
	for name, srv := range servers {
		serversJSON[name] = mcpServerJSON{
			Type:    srv.Type,
			Command: srv.Command,
			URL:     srv.URL,
			Args:    srv.Args,
			Env:     srv.Env,
		}
	}

	var (
		configPath string
		configData interface{}
	)

	switch b.Name {
	case "claude":
		configPath = filepath.Join(worktreePath, ".mcp.json")
		configData = map[string]interface{}{
			"mcpServers": serversJSON,
		}
	case "codex":
		configPath = filepath.Join(worktreePath, ".codex", "config.json")
		configData = map[string]interface{}{
			"mcpServers": serversJSON,
		}
	case "opencode":
		configPath = filepath.Join(worktreePath, "opencode.json")
		configData = map[string]interface{}{
			"mcpServers": serversJSON,
		}
	default:
		// Unknown backend; skip silently.
		return nil
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("writing MCP config to %s: %w", configPath, err)
	}

	return nil
}
