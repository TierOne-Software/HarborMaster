package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tierone/harbormaster/pkg/config"
	"github.com/tierone/harbormaster/pkg/lockfile"
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
		// Skip config loading for init command
		if cmd.Name() == "init" || cmd.Name() == "help" || cmd.Name() == "version" {
			return nil
		}

		// Load configuration
		var err error
		if cfgFile != "" {
			cfg, err = config.Load(cfgFile)
		} else {
			cfgPath, findErr := config.FindConfigFile()
			if findErr != nil {
				return fmt.Errorf("no config file found: %w\nRun 'hm init' to create one", findErr)
			}
			cfg, err = config.Load(cfgPath)
		}
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Override work directory if specified
		if workDir != "" {
			expandedPath, err := config.ExpandPath(workDir)
			if err != nil {
				return fmt.Errorf("invalid work directory: %w", err)
			}
			cfg.General.WorkDir = expandedPath
		}

		// Load lock file
		lockPath := getLockFilePath()
		lf, err = lockfile.Load(lockPath)
		if err != nil {
			return fmt.Errorf("failed to load lock file: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&workDir, "work-dir", "w", "", "override work directory")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "minimal output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
}

func getLockFilePath() string {
	if cfg != nil && cfg.Path() != "" {
		dir := getConfigDir()
		return dir + "/" + lockfile.LockFileName
	}
	cwd, _ := os.Getwd()
	return cwd + "/" + lockfile.LockFileName
}

func getConfigDir() string {
	if cfg != nil && cfg.Path() != "" {
		return cfg.Path()[:len(cfg.Path())-len(config.ConfigFileName)-1]
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

func Execute() error {
	return rootCmd.Execute()
}
