package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanz/island/internal/config"
)

// AgentDef defines how to invoke an AI coding agent.
type AgentDef struct {
	Name         string
	Command      string
	FirstRunArgs []string
	ResumeArgs   []string
	ExtraArgs    []string
	Env          map[string]string
	Model        string
	Permissions  string
	OutputFormat string
}

// AgentDefFromConfig creates an AgentDef from a config.AgentConfig.
func AgentDefFromConfig(name string, cfg config.AgentConfig) *AgentDef {
	return &AgentDef{
		Name:         name,
		Command:      cfg.Command,
		FirstRunArgs: cfg.FirstRunArgs,
		ResumeArgs:   cfg.ResumeArgs,
		ExtraArgs:    cfg.ExtraArgs,
		Env:          cfg.Env,
		Model:        cfg.Model,
		Permissions:  cfg.Permissions,
		OutputFormat: cfg.OutputFormat,
	}
}

// BuildArgs returns the full argument list for an invocation.
// Replaces {{prompt}} in the appropriate args template.
// If isResume, uses ResumeArgs; otherwise FirstRunArgs.
// Appends ExtraArgs. If Model is set, appends --model <model>.
// If Permissions is set, splits by spaces and prepends.
func (a *AgentDef) BuildArgs(prompt string, isResume bool) []string {
	var args []string

	// Prepend permissions flags if set.
	if a.Permissions != "" {
		parts := strings.Fields(a.Permissions)
		args = append(args, parts...)
	}

	// Choose the base args template.
	var baseArgs []string
	if isResume {
		baseArgs = a.ResumeArgs
	} else {
		baseArgs = a.FirstRunArgs
	}

	// Replace {{prompt}} placeholder in each arg.
	for _, arg := range baseArgs {
		args = append(args, strings.ReplaceAll(arg, "{{prompt}}", prompt))
	}

	// Append extra args.
	args = append(args, a.ExtraArgs...)

	// Append output format flag if set.
	if a.OutputFormat != "" {
		args = append(args, "--output-format", a.OutputFormat)
	}

	// Append model flag if set.
	if a.Model != "" {
		args = append(args, "--model", a.Model)
	}

	return args
}

// BuildEnv returns os.Environ() merged with the agent's extra env vars.
// Agent env vars override existing environment variables.
func (a *AgentDef) BuildEnv() []string {
	env := os.Environ()
	for k, v := range a.Env {
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

// SetupMCPConfig writes MCP server config into the worktree for this agent.
// Supported agents: "claude" (.mcp.json), "codex" (.codex/config.json),
// "opencode" (opencode.json). Unknown agents are silently skipped.
func (a *AgentDef) SetupMCPConfig(worktreePath string, servers map[string]config.MCPServer) error {
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

	switch a.Name {
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
		// Unknown agent; skip silently.
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
