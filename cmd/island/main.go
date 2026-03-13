package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hanz/island/internal/config"
	"github.com/hanz/island/internal/git"
	"github.com/hanz/island/internal/tui"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := newRootCmd()
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newCleanupCmd())
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		flagMaxConcurrent int
		flagBaseBranch    string
		flagAgent         string
		flagConfig        string
	)

	cmd := &cobra.Command{
		Use:           "island",
		Short:         "A TUI for orchestrating parallel AI coding agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// 1. Find repo root.
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("not a git repository")
			}

			// 2. Load config.
			var cfg *config.Config
			if flagConfig != "" {
				cfg, err = loadConfigFromFile(flagConfig)
				if err != nil {
					return fmt.Errorf("loading config from %s: %w", flagConfig, err)
				}
			} else {
				cfg, err = config.Load(repoRoot)
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
			}

			// Apply CLI flag overrides.
			if cmd.Flags().Changed("max-concurrent") {
				cfg.General.MaxConcurrent = flagMaxConcurrent
			}
			if cmd.Flags().Changed("base-branch") {
				cfg.General.BaseBranch = flagBaseBranch
			}
			if cmd.Flags().Changed("agent") {
				cfg.General.DefaultAgent = flagAgent
			}

			// 3. Validate config.
			if err := config.Validate(cfg); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}

			// 4-7. Run independent startup checks in parallel.
			var branchErr error
			var wg sync.WaitGroup

			wg.Add(4)
			go func() {
				defer wg.Done()
				branchErr = checkBranchExists(repoRoot, cfg.General.BaseBranch)
			}()
			go func() {
				defer wg.Done()
				for name, agentCfg := range cfg.Agents {
					if _, err := exec.LookPath(agentCfg.Command); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: agent %q command %q not found on PATH\n", name, agentCfg.Command)
					}
				}
			}()
			go func() {
				defer wg.Done()
				if cfg.General.AutoCleanup {
					mgr := git.NewManager(repoRoot, cfg.General.WorktreeDir)
					if err := mgr.Prune(ctx); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: auto-cleanup prune failed: %v\n", err)
					}
				}
			}()
			go func() {
				defer wg.Done()
				if err := git.EnsureGitignore(repoRoot); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to update .gitignore: %v\n", err)
				}
			}()
			wg.Wait()

			if branchErr != nil {
				return fmt.Errorf("base branch %q does not exist: %w", cfg.General.BaseBranch, branchErr)
			}

			// 8. Create App.
			app := tui.NewApp(cfg, repoRoot)

			// 9. Create tea.Program with alt screen.
			p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())

			// 10. Set program on app.
			app.SetProgram(p)

			// 11. Run.
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&flagMaxConcurrent, "max-concurrent", "c", 0, "override general.max_concurrent")
	cmd.Flags().StringVarP(&flagBaseBranch, "base-branch", "b", "", "override general.base_branch")
	cmd.Flags().StringVar(&flagAgent, "agent", "", "override general.default_agent")
	cmd.Flags().StringVar(&flagConfig, "config", "", "path to a specific config file")

	return cmd
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "init",
		Short:         "Initialize island configuration in the current repository",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check we're in a git repo.
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("not a git repository")
			}

			// Create .island/ directory.
			islandDir := filepath.Join(repoRoot, ".island")
			if err := os.MkdirAll(islandDir, 0755); err != nil {
				return fmt.Errorf("creating .island directory: %w", err)
			}

			// Write starter config.toml.
			configPath := filepath.Join(islandDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(starterConfig), 0644); err != nil {
				return fmt.Errorf("writing config.toml: %w", err)
			}
			fmt.Printf("Created %s\n", configPath)

			// Add .island/worktrees/ to .gitignore.
			if err := git.EnsureGitignore(repoRoot); err != nil {
				return fmt.Errorf("updating .gitignore: %w", err)
			}
			fmt.Printf("Ensured .island/ is in .gitignore\n")

			fmt.Printf("\nIsland initialized! Edit %s to customize.\n", configPath)
			return nil
		},
	}
}

func newCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "cleanup",
		Short:         "Prune stale worktrees and delete orphaned island/* branches",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Find repo root.
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("not a git repository")
			}

			// Load config.
			cfg, err := config.Load(repoRoot)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Prune stale worktrees.
			mgr := git.NewManager(repoRoot, cfg.General.WorktreeDir)
			if err := mgr.Prune(ctx); err != nil {
				return fmt.Errorf("pruning worktrees: %w", err)
			}
			fmt.Println("Pruned stale worktrees.")

			// List remaining worktrees to build a set of active branches.
			worktrees, err := mgr.List(ctx)
			if err != nil {
				return fmt.Errorf("listing worktrees: %w", err)
			}
			activeBranches := make(map[string]bool)
			for _, wt := range worktrees {
				if wt.Branch != "" {
					activeBranches[wt.Branch] = true
				}
			}

			// List island/* branches.
			listCmd := exec.CommandContext(ctx, "git", "branch", "--list", "island/*")
			listCmd.Dir = repoRoot
			var stdout, stderr bytes.Buffer
			listCmd.Stdout = &stdout
			listCmd.Stderr = &stderr
			if err := listCmd.Run(); err != nil {
				return fmt.Errorf("listing island branches: %w: %s", err, stderr.String())
			}

			// Delete orphaned branches.
			var cleaned int
			lines := strings.Split(stdout.String(), "\n")
			for _, line := range lines {
				branch := strings.TrimSpace(line)
				// git branch --list output may have a leading "* " for current branch.
				branch = strings.TrimPrefix(branch, "* ")
				if branch == "" {
					continue
				}
				if activeBranches[branch] {
					continue
				}
				// This branch has no corresponding worktree — delete it.
				delCmd := exec.CommandContext(ctx, "git", "branch", "-D", branch)
				delCmd.Dir = repoRoot
				var delStderr bytes.Buffer
				delCmd.Stderr = &delStderr
				if err := delCmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to delete branch %s: %v: %s\n", branch, err, delStderr.String())
					continue
				}
				fmt.Printf("Deleted orphaned branch: %s\n", branch)
				cleaned++
			}

			if cleaned == 0 {
				fmt.Println("No orphaned branches found.")
			} else {
				fmt.Printf("Cleaned up %d orphaned branch(es).\n", cleaned)
			}

			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print the island version",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("island %s\n", version)
		},
	}
}

// findRepoRoot returns the git repository root via `git rev-parse --show-toplevel`.
func findRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w: %s", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

// checkBranchExists verifies that the given branch exists in the repo.
func checkBranchExists(repoRoot, branch string) error {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git rev-parse --verify %s: %w: %s", branch, err, stderr.String())
	}
	return nil
}

// loadConfigFromFile loads defaults then overlays the given TOML file.
// config.Load expects a repo root with .island/config.toml, so we stage
// the file in a temp directory to reuse that loading logic.
func loadConfigFromFile(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "island-config-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	islandDir := filepath.Join(tmpDir, ".island")
	if err := os.MkdirAll(islandDir, 0755); err != nil {
		return nil, fmt.Errorf("creating temp .island dir: %w", err)
	}

	tmpConfig := filepath.Join(islandDir, "config.toml")
	if err := os.WriteFile(tmpConfig, data, 0644); err != nil {
		return nil, fmt.Errorf("writing temp config: %w", err)
	}

	return config.Load(tmpDir)
}

const starterConfig = `# Island configuration
# See https://github.com/hanz/island for documentation

# [general]
# default_agent = "claude"
# max_concurrent = 5
# base_branch = "main"
# worktree_dir = ".island/worktrees"
# output_buffer_size = 10000
# auto_cleanup = true

# [agents.claude]
# command = "claude"
# first_run_args = ["-p", "{{prompt}}"]
# resume_args = ["--continue", "-p", "{{prompt}}"]
# extra_args = []
# model = ""
# permissions = "--dangerously-skip-permissions"
# output_format = "stream-json"

# [hooks]
# pre_workspace_create = ""
# post_workspace_create = ""
# pre_merge = ""
# post_merge = ""

# [init]
# script = ""
`
