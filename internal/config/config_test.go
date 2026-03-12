package config

import (
	"testing"
)

func TestDefault(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, cfg *Config)
	}{
		{
			name: "default_backend is claude",
			check: func(t *testing.T, cfg *Config) {
				if cfg.General.DefaultBackend != "claude" {
					t.Errorf("expected default_backend=claude, got %q", cfg.General.DefaultBackend)
				}
			},
		},
		{
			name: "max_concurrent is 5",
			check: func(t *testing.T, cfg *Config) {
				if cfg.General.MaxConcurrent != 5 {
					t.Errorf("expected max_concurrent=5, got %d", cfg.General.MaxConcurrent)
				}
			},
		},
		{
			name: "base_branch is main",
			check: func(t *testing.T, cfg *Config) {
				if cfg.General.BaseBranch != "main" {
					t.Errorf("expected base_branch=main, got %q", cfg.General.BaseBranch)
				}
			},
		},
		{
			name: "worktree_dir is .island/worktrees",
			check: func(t *testing.T, cfg *Config) {
				if cfg.General.WorktreeDir != ".island/worktrees" {
					t.Errorf("expected worktree_dir=.island/worktrees, got %q", cfg.General.WorktreeDir)
				}
			},
		},
		{
			name: "output_buffer_size is 10000",
			check: func(t *testing.T, cfg *Config) {
				if cfg.General.OutputBufferSize != 10000 {
					t.Errorf("expected output_buffer_size=10000, got %d", cfg.General.OutputBufferSize)
				}
			},
		},
		{
			name: "auto_cleanup is true",
			check: func(t *testing.T, cfg *Config) {
				if !cfg.General.AutoCleanup {
					t.Error("expected auto_cleanup=true")
				}
			},
		},
		{
			name: "ui show_stderr is true",
			check: func(t *testing.T, cfg *Config) {
				if !cfg.UI.ShowStderr {
					t.Error("expected ui.show_stderr=true")
				}
			},
		},
		{
			name: "ui min_panel_width is 40",
			check: func(t *testing.T, cfg *Config) {
				if cfg.UI.MinPanelWidth != 40 {
					t.Errorf("expected ui.min_panel_width=40, got %d", cfg.UI.MinPanelWidth)
				}
			},
		},
		{
			name: "backends contain claude codex opencode",
			check: func(t *testing.T, cfg *Config) {
				for _, name := range []string{"claude", "codex", "opencode"} {
					if _, ok := cfg.Backends[name]; !ok {
						t.Errorf("expected backend %q to exist", name)
					}
				}
			},
		},
		{
			name: "templates contain test refactor fix review",
			check: func(t *testing.T, cfg *Config) {
				for _, name := range []string{"test", "refactor", "fix", "review"} {
					if _, ok := cfg.Templates[name]; !ok {
						t.Errorf("expected template %q to exist", name)
					}
				}
			},
		},
		{
			name: "default config passes validation",
			check: func(t *testing.T, cfg *Config) {
				if err := Validate(cfg); err != nil {
					t.Errorf("expected default config to be valid, got: %v", err)
				}
			},
		},
	}

	cfg := Default()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, cfg)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(cfg *Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(cfg *Config) {},
			wantErr: false,
		},
		{
			name: "unknown default backend",
			modify: func(cfg *Config) {
				cfg.General.DefaultBackend = "nonexistent"
			},
			wantErr: true,
		},
		{
			name: "empty default backend",
			modify: func(cfg *Config) {
				cfg.General.DefaultBackend = ""
			},
			wantErr: true,
		},
		{
			name: "max_concurrent zero",
			modify: func(cfg *Config) {
				cfg.General.MaxConcurrent = 0
			},
			wantErr: true,
		},
		{
			name: "max_concurrent negative",
			modify: func(cfg *Config) {
				cfg.General.MaxConcurrent = -1
			},
			wantErr: true,
		},
		{
			name: "empty base branch",
			modify: func(cfg *Config) {
				cfg.General.BaseBranch = ""
			},
			wantErr: true,
		},
		{
			name: "empty worktree dir",
			modify: func(cfg *Config) {
				cfg.General.WorktreeDir = ""
			},
			wantErr: true,
		},
		{
			name: "output_buffer_size zero",
			modify: func(cfg *Config) {
				cfg.General.OutputBufferSize = 0
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)
			err := Validate(cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        TemplateConfig
		description string
		want        string
	}{
		{
			name: "simple replacement",
			tmpl: TemplateConfig{
				Name:   "test",
				Prompt: "Write tests for: {{description}}",
			},
			description: "the user login flow",
			want:        "Write tests for: the user login flow",
		},
		{
			name: "multiple placeholders",
			tmpl: TemplateConfig{
				Name:   "custom",
				Prompt: "{{description}} needs {{description}}",
			},
			description: "attention",
			want:        "attention needs attention",
		},
		{
			name: "no placeholder",
			tmpl: TemplateConfig{
				Name:   "static",
				Prompt: "Run all tests",
			},
			description: "something",
			want:        "Run all tests",
		},
		{
			name: "empty description",
			tmpl: TemplateConfig{
				Name:   "test",
				Prompt: "Fix: {{description}}",
			},
			description: "",
			want:        "Fix: ",
		},
		{
			name: "default test template",
			tmpl: TemplateConfig{
				Name:   "test",
				Prompt: "Write comprehensive tests for: {{description}}",
			},
			description: "the HTTP handler package",
			want:        "Write comprehensive tests for: the HTTP handler package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyTemplate(tt.tmpl, tt.description)
			if got != tt.want {
				t.Errorf("ApplyTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}
