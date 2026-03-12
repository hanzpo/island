package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration structure for island.
type Config struct {
	General   GeneralConfig              `toml:"general"`
	Backends  map[string]BackendConfig   `toml:"backends"`
	MCP       map[string]MCPServer       `toml:"mcp"`
	Hooks     HooksConfig                `toml:"hooks"`
	Init      InitConfig                 `toml:"init"`
	Templates map[string]TemplateConfig  `toml:"templates"`
	UI        UIConfig                   `toml:"ui"`
}

// GeneralConfig holds general island settings.
type GeneralConfig struct {
	DefaultBackend   string `toml:"default_backend"`
	MaxConcurrent    int    `toml:"max_concurrent"`
	BaseBranch       string `toml:"base_branch"`
	WorktreeDir      string `toml:"worktree_dir"`
	OutputBufferSize int    `toml:"output_buffer_size"`
	AutoCleanup      bool   `toml:"auto_cleanup"`
}

// BackendConfig defines how to invoke an AI coding agent backend.
type BackendConfig struct {
	Command      string            `toml:"command"`
	FirstRunArgs []string          `toml:"first_run_args"`
	ResumeArgs   []string          `toml:"resume_args"`
	ExtraArgs    []string          `toml:"extra_args"`
	Env          map[string]string `toml:"env"`
	Model        string            `toml:"model"`
	Permissions  string            `toml:"permissions"`
}

// MCPServer defines an MCP server configuration.
type MCPServer struct {
	Type    string            `toml:"type"`
	Command string            `toml:"command"`
	URL     string            `toml:"url"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
}

// HooksConfig defines shell commands to run at lifecycle events.
type HooksConfig struct {
	PreWorkspaceCreate  string `toml:"pre_workspace_create"`
	PostWorkspaceCreate string `toml:"post_workspace_create"`
	PreMerge            string `toml:"pre_merge"`
	PostMerge           string `toml:"post_merge"`
	PreDiscard          string `toml:"pre_discard"`
	PostAgentTurn       string `toml:"post_agent_turn"`
}

// InitConfig holds workspace initialization settings.
type InitConfig struct {
	Script string `toml:"script"`
}

// TemplateConfig defines a prompt template.
type TemplateConfig struct {
	Name   string `toml:"name"`
	Prompt string `toml:"prompt"`
}

// UIConfig holds TUI display settings.
type UIConfig struct {
	Theme           string `toml:"theme"`
	ShowStderr      bool   `toml:"show_stderr"`
	TimestampOutput bool   `toml:"timestamp_output"`
	MinPanelWidth   int    `toml:"min_panel_width"`
}

// Default returns a Config with sane defaults, including pre-configured backends
// and prompt templates.
func Default() *Config {
	return &Config{
		General: GeneralConfig{
			DefaultBackend:   "claude",
			MaxConcurrent:    5,
			BaseBranch:       "main",
			WorktreeDir:      ".island/worktrees",
			OutputBufferSize: 10000,
			AutoCleanup:      true,
		},
		Backends: map[string]BackendConfig{
			"claude": {
				Command:      "claude",
				FirstRunArgs: []string{"-p", "{{prompt}}"},
				ResumeArgs:   []string{"--continue", "-p", "{{prompt}}"},
				ExtraArgs:    nil,
				Env:          map[string]string{},
			},
			"codex": {
				Command:      "codex",
				FirstRunArgs: []string{"exec", "{{prompt}}"},
				ResumeArgs:   []string{"exec", "resume", "--last", "{{prompt}}"},
				ExtraArgs:    []string{"--full-auto"},
				Env:          map[string]string{},
			},
			"opencode": {
				Command:      "opencode",
				FirstRunArgs: []string{"run", "{{prompt}}"},
				ResumeArgs:   []string{"run", "{{prompt}}"},
				ExtraArgs:    nil,
				Env:          map[string]string{},
			},
		},
		MCP:   map[string]MCPServer{},
		Hooks: HooksConfig{},
		Init:  InitConfig{},
		Templates: map[string]TemplateConfig{
			"test": {
				Name:   "test",
				Prompt: "Write comprehensive tests for: {{description}}",
			},
			"refactor": {
				Name:   "refactor",
				Prompt: "Refactor the following code: {{description}}",
			},
			"fix": {
				Name:   "fix",
				Prompt: "Fix the following issue: {{description}}",
			},
			"review": {
				Name:   "review",
				Prompt: "Review the following code and suggest improvements: {{description}}",
			},
		},
		UI: UIConfig{
			Theme:           "",
			ShowStderr:      true,
			TimestampOutput: false,
			MinPanelWidth:   40,
		},
	}
}

// Load reads the user config and project config, layering them on top of
// the defaults. Missing config files are silently ignored. Only parse errors
// are returned.
func Load(repoRoot string) (*Config, error) {
	cfg := Default()

	// Layer 1: user config (~/.config/island/config.toml)
	userDir, err := os.UserConfigDir()
	if err == nil {
		userPath := filepath.Join(userDir, "island", "config.toml")
		if err := decodeFileInto(userPath, cfg); err != nil {
			return nil, fmt.Errorf("parsing user config %s: %w", userPath, err)
		}
	}

	// Layer 2: project config (.island/config.toml)
	if repoRoot != "" {
		projectPath := filepath.Join(repoRoot, ".island", "config.toml")
		if err := decodeFileInto(projectPath, cfg); err != nil {
			return nil, fmt.Errorf("parsing project config %s: %w", projectPath, err)
		}
	}

	return cfg, nil
}

// decodeFileInto decodes a TOML file into the provided struct. If the file
// does not exist, it returns nil (silently ignored).
func decodeFileInto(path string, v interface{}) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	_, err = toml.DecodeFile(path, v)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// Validate checks the config for logical errors.
func Validate(cfg *Config) error {
	if cfg.General.DefaultBackend == "" {
		return fmt.Errorf("general.default_backend must be set")
	}
	if _, ok := cfg.Backends[cfg.General.DefaultBackend]; !ok {
		return fmt.Errorf("general.default_backend %q not found in backends", cfg.General.DefaultBackend)
	}
	if cfg.General.MaxConcurrent <= 0 {
		return fmt.Errorf("general.max_concurrent must be > 0, got %d", cfg.General.MaxConcurrent)
	}
	if cfg.General.BaseBranch == "" {
		return fmt.Errorf("general.base_branch must be set")
	}
	if cfg.General.WorktreeDir == "" {
		return fmt.Errorf("general.worktree_dir must be set")
	}
	if cfg.General.OutputBufferSize <= 0 {
		return fmt.Errorf("general.output_buffer_size must be > 0, got %d", cfg.General.OutputBufferSize)
	}
	return nil
}

// ApplyTemplate replaces {{description}} in the template prompt with the
// given description string.
func ApplyTemplate(tmpl TemplateConfig, description string) string {
	return strings.ReplaceAll(tmpl.Prompt, "{{description}}", description)
}
