package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
	"github.com/tierone/harbormaster/pkg/work"
)

var (
	// Global flags
	cfgFile string
	workDir string
	quiet   bool
	noColor bool

	// Loaded config and lockfile
	cfg *config.Config
	lf  *lockfile.LockFile

	// Inter-process workspace lock, held from PersistentPreRunE until the
	// command finishes so concurrent hm runs cannot interleave lockfile
	// writes or repository mutations.
	wsLock *lockfile.FileLock

	// Active work session (nil if none)
	ws *work.WorkSession
)

var rootCmd = &cobra.Command{
	Use:   "hm",
	Short: "Harbormaster - Multi-repository management tool",
	Long: `Harbormaster is a command-line tool for managing and synchronizing
multiple repositories. Define your repositories and projects in a config
file, then use 'hm sync' to keep them all up to date.

Use 'hm init' to initialize a new workspace, then 'hm sync' to
synchronize your repositories.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Honor --no-color before anything renders styled output.
		if noColor {
			lipgloss.SetColorProfile(termenv.Ascii)
		}

		// Skip config loading for commands that must work outside a workspace:
		// init, help, and cobra's completion machinery.
		if skipsConfigLoading(cmd) {
			return nil
		}

		// Resolve the config path without loading the file, so the workspace
		// lock can be acquired first: the config file is part of the state
		// the lock protects, and loading it before locking would let two
		// concurrent commands save stale copies over each other.
		cfgPath := cfgFile
		if cfgPath == "" {
			found, findErr := config.FindConfigFile()
			if findErr != nil {
				return fmt.Errorf("no config file found: %w\nRun 'hm init' to create one", findErr)
			}
			cfgPath = found
		}
		absCfgPath, err := filepath.Abs(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to resolve config path: %w", err)
		}

		// Serialize workspace access across hm processes; blocks until any
		// other hm run in this workspace finishes.
		lockPath := filepath.Join(filepath.Dir(absCfgPath), lockfile.LockFileName)
		wsLock, err = lockfile.Lock(lockPath)
		if err != nil {
			return fmt.Errorf("failed to lock workspace: %w", err)
		}

		// Load configuration. Read-only and repair commands tolerate a
		// config that fails validation, so a broken workspace can still be
		// inspected and fixed from the CLI.
		if allowsInvalidConfig(cmd) {
			var warnings []string
			cfg, warnings, err = config.LoadRelaxed(absCfgPath)
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
		} else {
			cfg, err = config.Load(absCfgPath)
		}
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Override work directory if specified; the override is runtime-only
		// and is never persisted by config saves.
		if workDir != "" {
			expandedPath, err := config.ExpandPath(workDir)
			if err != nil {
				return fmt.Errorf("invalid work directory: %w", err)
			}
			cfg.SetWorkDirOverride(expandedPath)
		}

		// Load lock file
		lf, err = lockfile.Load(lockPath)
		if err != nil {
			if allowsInvalidConfig(cmd) {
				fmt.Fprintf(os.Stderr, "warning: ignoring unreadable lock file: %v\n", err)
				lf = lockfile.New()
			} else {
				return fmt.Errorf("failed to load lock file: %w\n"+
					"If the file is corrupt, run 'hm init --force' to reset it (a backup is kept), or delete %s and run 'hm sync' to regenerate it", err, lockPath)
			}
		}

		// Load work session if one exists
		workPath := getWorkFilePath()
		if work.FileExists(workPath) {
			ws, err = work.Load(workPath)
			if err != nil {
				return fmt.Errorf("failed to load work session: %w", err)
			}
		}

		return nil
	},
	// Runs only after a successful RunE; on error paths the OS releases the
	// flock at process exit.
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		_ = wsLock.Unlock()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&workDir, "work-dir", "w", "", "override work directory")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "minimal output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
}

// allowsInvalidConfig reports whether cmd should run even when the config
// (or lock file) fails validation, so users can inspect and repair a broken
// workspace with the CLI instead of hand-editing TOML. Mutating commands in
// this set still validate the final state before saving.
func allowsInvalidConfig(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "status", "remove", "remove-repo",
		"list", "repos", "projects", "tags":
		return true
	}
	return false
}

// skipsConfigLoading reports whether cmd should run without a workspace
// (no config, lock file, or work session loaded).
func skipsConfigLoading(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "init", "help", "completion",
		cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	}
	// Shell completion subcommands: `hm completion bash|zsh|fish|powershell`.
	if cmd.Parent() != nil && cmd.Parent().Name() == "completion" {
		return true
	}
	return false
}

func getLockFilePath() string {
	return filepath.Join(getConfigDir(), lockfile.LockFileName)
}

// getConfigDir returns the directory containing the loaded config file,
// falling back to the current working directory.
func getConfigDir() string {
	if cfg != nil && cfg.Path() != "" {
		abs, err := filepath.Abs(cfg.Path())
		if err != nil {
			return filepath.Dir(cfg.Path())
		}
		return filepath.Dir(abs)
	}
	cwd, _ := os.Getwd()
	return cwd
}

func saveLockFile() error {
	if lf == nil {
		return nil
	}
	return lf.Save(getLockFilePath())
}

func getWorkFilePath() string {
	return filepath.Join(getConfigDir(), work.WorkFileName)
}

func saveWorkFile() error {
	if ws == nil {
		return nil
	}
	return ws.Save(getWorkFilePath())
}

func deleteWorkFile() error {
	path := getWorkFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

func Execute() error {
	return rootCmd.Execute()
}
